package moysklad

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"github.com/sirupsen/logrus"

	"upd-loader-go/internal/models"
)

// APIError represents a MoySkald API error
type APIError struct {
	Message string
}

func (e *APIError) Error() string {
	return e.Message
}

// API represents MoySkald API client
type API struct {
	baseURL        string
	token          string
	organizationID string
	client         *http.Client
	logger         *logrus.Logger
}

// NewAPI creates a new MoySkald API client
func NewAPI(baseURL, token, organizationID string, logger *logrus.Logger) *API {
	return &API{
		baseURL:        baseURL,
		token:          token,
		organizationID: organizationID,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger: logger,
	}
}

// makeRequest performs HTTP request with logging
func (api *API) makeRequest(method, endpoint string, data interface{}, params map[string]string) (*http.Response, error) {
	fullURL := api.baseURL + endpoint

	// Add query parameters
	if len(params) > 0 {
		u, err := url.Parse(fullURL)
		if err != nil {
			return nil, err
		}
		q := u.Query()
		for k, v := range params {
			q.Set(k, v)
		}
		u.RawQuery = q.Encode()
		fullURL = u.String()
	}

	var body io.Reader
	if data != nil {
		jsonData, err := json.Marshal(data)
		if err != nil {
			return nil, err
		}
		body = bytes.NewBuffer(jsonData)
	}

	req, err := http.NewRequest(method, fullURL, body)
	if err != nil {
		return nil, err
	}

	// Set headers
	req.Header.Set("Authorization", "Bearer "+api.token)
	req.Header.Set("Content-Type", "application/json;charset=utf-8")
	req.Header.Set("Accept", "application/json;charset=utf-8")

	start := time.Now()
	resp, err := api.client.Do(req)
	duration := time.Since(start)

	// Log request
	api.logRequest(method, endpoint, resp, duration, data)

	return resp, err
}

// logRequest logs HTTP requests
func (api *API) logRequest(method, endpoint string, resp *http.Response, duration time.Duration, requestData interface{}) {
	logData := map[string]interface{}{
		"method":      method,
		"endpoint":    endpoint,
		"duration_ms": duration.Milliseconds(),
	}

	if resp != nil {
		logData["status_code"] = resp.StatusCode

		if resp.StatusCode == 200 {
			api.logger.WithFields(logData).Infof("MoySkald API: %s %s -> %d (%dms)", method, endpoint, resp.StatusCode, duration.Milliseconds())
		} else if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			api.logger.WithFields(logData).Errorf("MoySkald API: Client error %s %s -> %d", method, endpoint, resp.StatusCode)
		} else if resp.StatusCode >= 500 {
			api.logger.WithFields(logData).Errorf("MoySkald API: Server error %s %s -> %d", method, endpoint, resp.StatusCode)
		} else {
			api.logger.WithFields(logData).Warnf("MoySkald API: %s %s -> %d (%dms)", method, endpoint, resp.StatusCode, duration.Milliseconds())
		}
	} else {
		api.logger.WithFields(logData).Errorf("MoySkald API: Request failed %s %s", method, endpoint)
	}
}

// VerifyToken verifies API token validity
func (api *API) VerifyToken() bool {
	resp, err := api.makeRequest("GET", "/context/employee", nil, nil)
	if err != nil {
		api.logger.Errorf("Token verification error: %v", err)
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == 200
}

// VerifyAPIAccess verifies API access and returns detailed information
func (api *API) VerifyAPIAccess() map[string]interface{} {
	api.logger.Info("Verifying MoySkald API access...")

	// Check basic API access
	resp, err := api.makeRequest("GET", "/context/employee", nil, nil)
	if err != nil {
		return map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("Network error: %v", err),
			"details": "Check internet connection and api.moysklad.ru availability",
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("API access error: %d", resp.StatusCode),
			"details": string(body),
		}
	}

	var employeeData map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&employeeData); err != nil {
		return map[string]interface{}{
			"success": false,
			"error":   "Failed to decode employee data",
			"details": err.Error(),
		}
	}

	// Get organization information
	orgResp, err := api.makeRequest("GET", "/entity/organization", nil, nil)
	if err != nil {
		return map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("Failed to get organizations: %v", err),
		}
	}
	defer orgResp.Body.Close()

	if orgResp.StatusCode != 200 {
		body, _ := io.ReadAll(orgResp.Body)
		return map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("No access to organizations: %d", orgResp.StatusCode),
			"details": string(body),
		}
	}

	var orgData map[string]interface{}
	if err := json.NewDecoder(orgResp.Body).Decode(&orgData); err != nil {
		return map[string]interface{}{
			"success": false,
			"error":   "Failed to decode organization data",
			"details": err.Error(),
		}
	}

	organizations, ok := orgData["rows"].([]interface{})
	if !ok || len(organizations) == 0 {
		return map[string]interface{}{
			"success": false,
			"error":   "No organizations found",
			"details": "No available organizations in MoySkald account",
		}
	}

	// Check permissions
	permissions := api.checkPermissions()

	mainOrg := organizations[0].(map[string]interface{})

	return map[string]interface{}{
		"success": true,
		"employee": map[string]interface{}{
			"name":  employeeData["name"],
			"email": employeeData["email"],
		},
		"organization": map[string]interface{}{
			"name": mainOrg["name"],
			"inn":  mainOrg["inn"],
			"id":   mainOrg["id"],
		},
		"permissions": permissions,
		"api_info": map[string]interface{}{
			"base_url":         api.baseURL,
			"response_time_ms": "< 10000",
		},
	}
}

// checkPermissions checks various API permissions
func (api *API) checkPermissions() map[string]interface{} {
	permissions := map[string]interface{}{
		"organizations_count": 0,
		"stores_count":        0,
	}

	// Check invoice creation access
	resp, err := api.makeRequest("GET", "/entity/factureout", nil, nil)
	permissions["can_create_invoices"] = err == nil && resp != nil && resp.StatusCode == 200
	if resp != nil {
		resp.Body.Close()
	}

	// Check counterparty access
	resp, err = api.makeRequest("GET", "/entity/counterparty", nil, nil)
	permissions["can_access_counterparties"] = err == nil && resp != nil && resp.StatusCode == 200
	if resp != nil {
		resp.Body.Close()
	}

	// Check stores access
	resp, err = api.makeRequest("GET", "/entity/store", nil, nil)
	canAccessStores := err == nil && resp != nil && resp.StatusCode == 200
	permissions["can_access_stores"] = canAccessStores
	if resp != nil && canAccessStores {
		var storeData map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&storeData)
		if stores, ok := storeData["rows"].([]interface{}); ok {
			permissions["stores_count"] = len(stores)
		}
		resp.Body.Close()
	}

	return permissions
}

// CreateInvoiceFromUPD creates invoice and demand from UPD document
func (api *API) CreateInvoiceFromUPD(updDocument *models.UPDDocument) (map[string]interface{}, error) {
	api.logger.Infof("Creating documents for UPD: %s", updDocument.DocumentID())

	// Find supplier organization by INN
	supplierOrg, err := api.findOrganizationByINN(updDocument.Content.Seller.INN)
	if err != nil {
		return nil, &APIError{Message: fmt.Sprintf("Supplier organization with INN %s not found in MoySkald", updDocument.Content.Seller.INN)}
	}

	// Get or create buyer counterparty
	buyerCounterparty, err := api.getOrCreateCounterparty(updDocument.Content.Buyer)
	if err != nil {
		return nil, err
	}

	// Step 1: Create demand (shipment) as base document
	api.logger.Info("Creating demand as base document...")
	demand, err := api.createDemand(updDocument, supplierOrg, buyerCounterparty)
	if err != nil {
		return nil, err
	}

	// Step 2: Create invoice based on demand
	api.logger.Info("Creating invoice based on demand...")
	invoiceData := api.mapUPDToFactureOut(updDocument, supplierOrg, buyerCounterparty, demand)

	resp, err := api.makeRequest("POST", "/entity/factureout", invoiceData, nil)
	if err != nil {
		return nil, &APIError{Message: fmt.Sprintf("Network error creating invoice: %v", err)}
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		var result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return nil, &APIError{Message: fmt.Sprintf("Failed to decode invoice response: %v", err)}
		}

		api.logger.Infof("Invoice successfully created: %s", result["id"])
		return map[string]interface{}{
			"factureout": result,
			"demand":     demand,
			"success":    true,
		}, nil
	}

	body, _ := io.ReadAll(resp.Body)
	errorMsg := fmt.Sprintf("Error creating invoice: %d - %s", resp.StatusCode, string(body))
	api.logger.Error(errorMsg)
	return nil, &APIError{Message: errorMsg}
}

// findOrganizationByINN finds organization by INN
func (api *API) findOrganizationByINN(inn string) (map[string]interface{}, error) {
	params := map[string]string{"filter": "inn=" + inn}
	resp, err := api.makeRequest("GET", "/entity/organization", nil, params)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		var data map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
			return nil, err
		}

		if organizations, ok := data["rows"].([]interface{}); ok && len(organizations) > 0 {
			org := organizations[0].(map[string]interface{})
			api.logger.Infof("Found organization by INN %s: %s", inn, org["name"])
			return org, nil
		}
	}

	api.logger.Warningf("Organization with INN %s not found", inn)
	return nil, fmt.Errorf("organization not found")
}

// getOrCreateCounterparty gets existing or creates new counterparty
func (api *API) getOrCreateCounterparty(buyer models.Organization) (map[string]interface{}, error) {
	// Search by INN
	params := map[string]string{"filter": "inn=" + buyer.INN}
	resp, err := api.makeRequest("GET", "/entity/counterparty", nil, params)
	if err != nil {
		return nil, &APIError{Message: fmt.Sprintf("Network error searching counterparty: %v", err)}
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		var data map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&data); err == nil {
			if counterparties, ok := data["rows"].([]interface{}); ok && len(counterparties) > 0 {
				counterparty := counterparties[0].(map[string]interface{})
				api.logger.Infof("Found existing counterparty: %s", counterparty["name"])
				return counterparty, nil
			}
		}
	}

	// Create new counterparty
	api.logger.Infof("Creating new counterparty: %s", buyer.Name)

	// Determine counterparty type by INN length
	isIndividual := len(buyer.INN) == 12

	counterpartyData := map[string]interface{}{
		"name":        buyer.Name,
		"inn":         buyer.INN,
		"companyType": "legal",
	}

	if isIndividual {
		counterpartyData["companyType"] = "individual"
		api.logger.Infof("Creating counterparty as individual entrepreneur (INN: %s)", buyer.INN)
	} else {
		if buyer.KPP != "" {
			counterpartyData["kpp"] = buyer.KPP
		}
		api.logger.Infof("Creating counterparty as legal entity (INN: %s, KPP: %s)", buyer.INN, buyer.KPP)
	}

	resp, err = api.makeRequest("POST", "/entity/counterparty", counterpartyData, nil)
	if err != nil {
		return nil, &APIError{Message: fmt.Sprintf("Network error creating counterparty: %v", err)}
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		var result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return nil, &APIError{Message: fmt.Sprintf("Failed to decode counterparty response: %v", err)}
		}

		api.logger.Infof("Counterparty successfully created: %s", result["name"])
		return result, nil
	}

	body, _ := io.ReadAll(resp.Body)
	errorMsg := fmt.Sprintf("Error creating counterparty: %d - %s", resp.StatusCode, string(body))
	api.logger.Error(errorMsg)
	return nil, &APIError{Message: errorMsg}
}

// createDemand creates demand (shipment) document
func (api *API) createDemand(updDocument *models.UPDDocument, organization, counterparty map[string]interface{}) (map[string]interface{}, error) {
	content := updDocument.Content

	// Format date for MoySkald: YYYY-MM-DD HH:MM:SS.sss
	momentStr := content.InvoiceDate.Format("2006-01-02 15:04:05.000")

	// Find customer invoice by requisite number
	customerInvoice, err := api.findCustomerInvoice(content.RequisiteNumber, counterparty)
	if err != nil {
		return nil, &APIError{Message: fmt.Sprintf("Customer invoice with number '%s' not found.\nCreate invoice with specified number and try again.", content.RequisiteNumber)}
	}

	// Get store from customer invoice
	store, err := api.getStoreFromInvoice(customerInvoice)
	if err != nil {
		return nil, &APIError{Message: fmt.Sprintf("Store not specified in customer invoice '%s'.\nSpecify store in invoice and try again.", customerInvoice["name"])}
	}

	api.logger.Infof("Final store for demand: %s (ID: %s)", store["name"], store["id"])

	// Create demand data
	demandData := map[string]interface{}{
		"name":   "О" + content.InvoiceNumber, // Prefix "О" + UPD number
		"moment": momentStr,
		"organization": map[string]interface{}{
			"meta": organization["meta"],
		},
		"agent": map[string]interface{}{
			"meta": counterparty["meta"],
		},
		"store": map[string]interface{}{
			"meta": store["meta"],
		},
		"vatEnabled":  true,
		"vatIncluded": true,
		"positions":   []interface{}{},
	}

	// Link to customer invoice if found
	if customerInvoice != nil {
		demandData["invoicesOut"] = []interface{}{
			map[string]interface{}{
				"meta": customerInvoice["meta"],
			},
		}
	}

	// Add positions
	positions, err := api.createPositionsFromUPD(&content, customerInvoice)
	if err != nil {
		return nil, err
	}
	demandData["positions"] = positions

	// Create demand
	resp, err := api.makeRequest("POST", "/entity/demand", demandData, nil)
	if err != nil {
		return nil, &APIError{Message: fmt.Sprintf("Network error creating demand: %v", err)}
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		var result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return nil, &APIError{Message: fmt.Sprintf("Failed to decode demand response: %v", err)}
		}

		api.logger.Infof("Demand successfully created: %s", result["id"])
		return result, nil
	}

	body, _ := io.ReadAll(resp.Body)
	errorMsg := fmt.Sprintf("Error creating demand: %d - %s", resp.StatusCode, string(body))
	api.logger.Error(errorMsg)
	return nil, &APIError{Message: errorMsg}
}

// mapUPDToFactureOut converts UPD to MoySkald invoice format
func (api *API) mapUPDToFactureOut(updDocument *models.UPDDocument, organization, counterparty, demand map[string]interface{}) map[string]interface{} {
	content := updDocument.Content

	// Format date for MoySkald: YYYY-MM-DD HH:MM:SS.sss
	momentStr := content.InvoiceDate.Format("2006-01-02 15:04:05.000")

	invoiceData := map[string]interface{}{
		"name":   content.InvoiceNumber, // UPD number as is
		"moment": momentStr,
		"organization": map[string]interface{}{
			"meta": organization["meta"],
		},
		"agent": map[string]interface{}{
			"meta": counterparty["meta"],
		},
		"vatEnabled":  true,
		"vatIncluded": true,
		"demands": []interface{}{
			map[string]interface{}{
				"meta": demand["meta"],
			},
		},
		"positions": []interface{}{},
	}

	// Add positions (reuse same logic as demand)
	customerInvoice, _ := api.findCustomerInvoice(content.RequisiteNumber, nil)
	positions, _ := api.createPositionsFromUPD(&content, customerInvoice)
	invoiceData["positions"] = positions

	api.logger.Debugf("Creating invoice: %s based on demand %s", invoiceData["name"], demand["id"])

	return invoiceData
}

// createPositionsFromUPD creates document positions from UPD
func (api *API) createPositionsFromUPD(content *models.UPDContent, customerInvoice map[string]interface{}) ([]interface{}, error) {
	var positions []interface{}
	var missingItems []string

	// Get positions from invoice for price matching
	invoicePositions := make(map[string]int64)
	if customerInvoice != nil {
		invoicePositions = api.getInvoicePositions(customerInvoice)
	}

	// Add positions from UPD
	for _, item := range content.Items {
		// Find product by article first
		var product map[string]interface{}
		if item.Article != "" {
			api.logger.Infof("Searching product by article: %s", item.Article)
			product = api.findProductByArticle(item.Article)
			if product != nil {
				api.logger.Infof("✅ Product found by article %s: %s (ID: %s)", item.Article, product["name"], product["id"])
			} else {
				api.logger.Warningf("❌ Product not found by article: %s", item.Article)
			}
		}

		// If not found by article, search by name
		if product == nil {
			api.logger.Infof("Searching product by name: %s", item.Name)
			product = api.findProduct(item.Name)
			if product != nil {
				api.logger.Infof("✅ Product found by name: %s (ID: %s)", product["name"], product["id"])
			} else {
				api.logger.Warningf("❌ Product not found by name: %s", item.Name)
			}
		}

		if product != nil {
			// Determine price: from invoice first, then from UPD
			priceKopecks := int64(item.Price.Mul(decimal.NewFromInt(100)).IntPart())

			// Search price in invoice by article
			if item.Article != "" {
				if invoicePrice, exists := invoicePositions["article:"+item.Article]; exists && invoicePrice > 0 {
					priceKopecks = invoicePrice
					api.logger.Infof("Using price from invoice by article %s: %.2f rub", item.Article, float64(priceKopecks)/100)
				}
			}
			// If not found by article, search by name
			if priceKopecks == int64(item.Price.Mul(decimal.NewFromInt(100)).IntPart()) {
				if invoicePrice, exists := invoicePositions["name:"+item.Name]; exists && invoicePrice > 0 {
					priceKopecks = invoicePrice
					api.logger.Infof("Using price from invoice by name '%s': %.2f rub", item.Name, float64(priceKopecks)/100)
				} else {
					api.logger.Warningf("Price for product '%s' not found in invoice, using UPD price: %.2f rub", item.Name, float64(priceKopecks)/100)
				}
			}

			position := map[string]interface{}{
				"quantity": item.Quantity.InexactFloat64(),
				"price":    priceKopecks,
				"assortment": map[string]interface{}{
					"meta": product["meta"],
				},
				"vat": api.getVATRate(item.VATRate),
			}
			positions = append(positions, position)
		} else {
			articleInfo := item.Article
			if articleInfo == "" {
				articleInfo = "не указан"
			}
			missingItems = append(missingItems, fmt.Sprintf("%s (артикул: %s)", item.Name, articleInfo))
		}
	}

	// If there are missing items, return error
	if len(missingItems) > 0 {
		errorMsg := fmt.Sprintf("The following products from UPD are not found in MoySkald:\n• %s\n\nCreate these products in MoySkald manually and retry UPD upload.", strings.Join(missingItems, "\n• "))
		return nil, &APIError{Message: errorMsg}
	}

	// If no positions from UPD, use any available service
	if len(positions) == 0 {
		totalPriceKopecks := int64(1000 * 100) // 1000 rub default
		if content.TotalWithVAT.GreaterThan(decimal.Zero) {
			totalPriceKopecks = int64(content.TotalWithVAT.Mul(decimal.NewFromInt(100)).IntPart())
		}

		service := api.getAnyAvailableService()
		if service == nil {
			return nil, &APIError{Message: "No available services in MoySkald to create document position.\nCreate at least one service in MoySkald and try again."}
		}

		positions = append(positions, map[string]interface{}{
			"quantity": 1,
			"price":    totalPriceKopecks,
			"assortment": map[string]interface{}{
				"meta": service["meta"],
			},
			"vat": 18,
		})
	}

	return positions, nil
}

// getInvoicePositions gets positions from invoice for price matching
func (api *API) getInvoicePositions(customerInvoice map[string]interface{}) map[string]int64 {
	positions := make(map[string]int64)

	// Get full invoice information with positions
	if meta, ok := customerInvoice["meta"].(map[string]interface{}); ok {
		if href, ok := meta["href"].(string); ok {
			resp, err := api.makeRequest("GET", strings.TrimPrefix(href, api.baseURL)+"?expand=positions.assortment", nil, nil)
			if err != nil {
				api.logger.Errorf("Error getting invoice positions: %v", err)
				return positions
			}
			defer resp.Body.Close()

			if resp.StatusCode == 200 {
				var invoiceData map[string]interface{}
				if err := json.NewDecoder(resp.Body).Decode(&invoiceData); err == nil {
					if positionsData, ok := invoiceData["positions"]; ok {
						api.parseInvoicePositions(positionsData, positions)
					}
				}
			}
		}
	}

	api.logger.Infof("Loaded %d positions from invoice for price matching", len(positions))
	return positions
}

// parseInvoicePositions parses positions from invoice data
func (api *API) parseInvoicePositions(positionsData interface{}, positions map[string]int64) {
	switch data := positionsData.(type) {
	case map[string]interface{}:
		if rows, ok := data["rows"].([]interface{}); ok {
			for _, pos := range rows {
				api.parsePosition(pos, positions)
			}
		} else if href, ok := data["meta"].(map[string]interface{})["href"].(string); ok {
			// Load positions separately
			resp, err := api.makeRequest("GET", strings.TrimPrefix(href, api.baseURL), nil, nil)
			if err == nil {
				defer resp.Body.Close()
				if resp.StatusCode == 200 {
					var positionsResult map[string]interface{}
					if json.NewDecoder(resp.Body).Decode(&positionsResult) == nil {
						if rows, ok := positionsResult["rows"].([]interface{}); ok {
							for _, pos := range rows {
								api.parsePosition(pos, positions)
							}
						}
					}
				}
			}
		}
	case []interface{}:
		for _, pos := range data {
			api.parsePosition(pos, positions)
		}
	}
}

// parsePosition parses individual position
func (api *API) parsePosition(pos interface{}, positions map[string]int64) {
	if position, ok := pos.(map[string]interface{}); ok {
		if assortment, ok := position["assortment"].(map[string]interface{}); ok {
			productName, _ := assortment["name"].(string)
			productArticle, _ := assortment["article"].(string)
			price, _ := position["price"].(float64)

			if productArticle != "" {
				positions["article:"+productArticle] = int64(price)
			}
			if productName != "" {
				positions["name:"+productName] = int64(price)
			}
		}
	}
}

// findProduct finds product by name
func (api *API) findProduct(productName string) map[string]interface{} {
	params := map[string]string{"filter": "name=" + productName}
	resp, err := api.makeRequest("GET", "/entity/product", nil, params)
	if err != nil {
		api.logger.Errorf("Error searching product: %v", err)
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		var data map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&data); err == nil {
			if products, ok := data["rows"].([]interface{}); ok && len(products) > 0 {
				product := products[0].(map[string]interface{})
				api.logger.Debugf("Found product: %s", product["name"])
				return product
			}
		}
	}

	api.logger.Warningf("Product '%s' not found in MoySkald", productName)
	return nil
}

// findProductByArticle finds product by article
func (api *API) findProductByArticle(article string) map[string]interface{} {
	params := map[string]string{"filter": "article=" + article}
	resp, err := api.makeRequest("GET", "/entity/product", nil, params)
	if err != nil {
		api.logger.Errorf("Error searching product by article: %v", err)
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		var data map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&data); err == nil {
			if products, ok := data["rows"].([]interface{}); ok && len(products) > 0 {
				product := products[0].(map[string]interface{})
				api.logger.Debugf("Found product by article %s: %s", article, product["name"])
				return product
			}
		}
	}

	api.logger.Debugf("Product with article %s not found", article)
	return nil
}

// getAnyAvailableService gets any available service
func (api *API) getAnyAvailableService() map[string]interface{} {
	resp, err := api.makeRequest("GET", "/entity/service", nil, nil)
	if err != nil {
		api.logger.Errorf("Error getting services: %v", err)
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		var data map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&data); err == nil {
			if services, ok := data["rows"].([]interface{}); ok && len(services) > 0 {
				service := services[0].(map[string]interface{})
				api.logger.Debugf("Using available service: %s", service["name"])
				return service
			}
		}
	}

	api.logger.Warning("No available services in MoySkald")
	return nil
}

// findCustomerInvoice finds customer invoice by requisite number
func (api *API) findCustomerInvoice(requisiteNumber string, counterparty map[string]interface{}) (map[string]interface{}, error) {
	if requisiteNumber == "" {
		api.logger.Debug("Requisite number not found")
		return nil, fmt.Errorf("requisite number not provided")
	}

	api.logger.Infof("Searching supplier invoice with number: %s", requisiteNumber)

	// Search patterns for invoice
	searchPatterns := []string{
		"name=" + requisiteNumber,
		"name~" + requisiteNumber,
		"description~" + requisiteNumber,
	}

	for _, pattern := range searchPatterns {
		api.logger.Debugf("Searching invoice with filter: %s", pattern)

		params := map[string]string{"filter": pattern}
		resp, err := api.makeRequest("GET", "/entity/invoiceout", nil, params)
		if err != nil {
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode == 200 {
			var data map[string]interface{}
			if err := json.NewDecoder(resp.Body).Decode(&data); err == nil {
				if invoices, ok := data["rows"].([]interface{}); ok && len(invoices) > 0 {
					invoice := invoices[0].(map[string]interface{})

					// Get full invoice information
					if meta, ok := invoice["meta"].(map[string]interface{}); ok {
						if href, ok := meta["href"].(string); ok {
							fullResp, err := api.makeRequest("GET", strings.TrimPrefix(href, api.baseURL), nil, nil)
							if err == nil {
								defer fullResp.Body.Close()
								if fullResp.StatusCode == 200 {
									var invoiceData map[string]interface{}
									if json.NewDecoder(fullResp.Body).Decode(&invoiceData) == nil {
										agentName := "unknown"
										if agent, ok := invoiceData["agent"].(map[string]interface{}); ok {
											if name, ok := agent["name"].(string); ok {
												agentName = name
											}
										}

										api.logger.Infof("Found supplier invoice: %s (counterparty: %s, filter: %s)", invoice["name"], agentName, pattern)
										return invoiceData, nil
									}
								}
							}
						}
					}
				}
			}
		}
	}

	api.logger.Warningf("Supplier invoice with number %s not found", requisiteNumber)
	return nil, fmt.Errorf("invoice not found")
}

// getStoreFromInvoice gets store from customer invoice
func (api *API) getStoreFromInvoice(customerInvoice map[string]interface{}) (map[string]interface{}, error) {
	if customerInvoice == nil {
		return nil, fmt.Errorf("customer invoice is nil")
	}

	api.logger.Infof("Found customer invoice: %s", customerInvoice["name"])

	// Look for store in invoice
	if store, ok := customerInvoice["store"]; ok && store != nil {
		if storeMap, ok := store.(map[string]interface{}); ok {
			storeName, _ := storeMap["name"].(string)
			storeID, _ := storeMap["id"].(string)

			// If store doesn't have direct name/id, it might be a meta reference
			if storeName == "" && storeID == "" {
				if meta, ok := storeMap["meta"].(map[string]interface{}); ok {
					if href, ok := meta["href"].(string); ok {
						// Get full store information
						storeResp, err := api.makeRequest("GET", strings.TrimPrefix(href, api.baseURL), nil, nil)
						if err == nil {
							defer storeResp.Body.Close()
							if storeResp.StatusCode == 200 {
								var storeData map[string]interface{}
								if json.NewDecoder(storeResp.Body).Decode(&storeData) == nil {
									storeName, _ = storeData["name"].(string)
									storeID, _ = storeData["id"].(string)
									api.logger.Debugf("Got full store information: %s (ID: %s)", storeName, storeID)
									return storeData, nil
								}
							}
						}
					}
				}
			}

			if storeName != "" || storeID != "" {
				api.logger.Infof("Store from invoice: %s (ID: %s)", storeName, storeID)
				return storeMap, nil
			}
		}
	}

	return nil, fmt.Errorf("store not specified in invoice")
}

// getVATRate converts VAT rate string to numeric value
func (api *API) getVATRate(vatRateStr string) int {
	if vatRateStr == "" {
		return 18
	}

	// Extract number from string like "18%" or "20%"
	re := regexp.MustCompile(`(\d+)`)
	matches := re.FindStringSubmatch(vatRateStr)
	if len(matches) > 1 {
		if rate, err := strconv.Atoi(matches[1]); err == nil {
			return rate
		}
	}

	return 18 // Default
}

// GetInvoiceURL returns invoice URL in MoySkald web interface
func (api *API) GetInvoiceURL(invoiceID string) string {
	return fmt.Sprintf("https://online.moysklad.ru/app/#factureout/edit?id=%s", invoiceID)
}

// GetDemandURL returns demand URL in MoySkald web interface
func (api *API) GetDemandURL(demandID string) string {
	return fmt.Sprintf("https://online.moysklad.ru/app/#demand/edit?id=%s", demandID)
}

// GetInvoiceInfo gets invoice information
func (api *API) GetInvoiceInfo(invoiceID string) (map[string]interface{}, error) {
	resp, err := api.makeRequest("GET", "/entity/factureout/"+invoiceID, nil, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		var data map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
			return nil, err
		}
		return data, nil
	}

	api.logger.Errorf("Error getting invoice information: %d", resp.StatusCode)
	return nil, fmt.Errorf("failed to get invoice info")
}