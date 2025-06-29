package processor

import (
	"fmt"
	"io/ioutil"
	"strings"

	"github.com/sirupsen/logrus"

	"upd-loader-go/internal/config"
	"upd-loader-go/internal/models"
	"upd-loader-go/internal/moysklad"
	"upd-loader-go/internal/parser"
)

// UPDProcessor handles UPD document processing
type UPDProcessor struct {
	config     *config.Config
	parser     *parser.UPDParser
	moyskladAPI *moysklad.API
	logger     *logrus.Logger
}

// NewUPDProcessor creates a new UPD processor
func NewUPDProcessor(cfg *config.Config, logger *logrus.Logger) *UPDProcessor {
	updParser := parser.NewUPDParser(cfg.UPDEncoding, logger)
	moyskladAPI := moysklad.NewAPI(cfg.MoySkladAPIURL, cfg.MoySkladAPIToken, cfg.MoySkladOrganizationID, logger)

	return &UPDProcessor{
		config:      cfg,
		parser:      updParser,
		moyskladAPI: moyskladAPI,
		logger:      logger,
	}
}

// ProcessUPDFile processes UPD file
func (p *UPDProcessor) ProcessUPDFile(fileContent []byte, filename string) *models.ProcessingResult {
	var tempZipPath string

	defer func() {
		if tempZipPath != "" {
			p.cleanupTempFiles(tempZipPath)
		}
	}()

	p.logger.Infof("Starting UPD file processing: %s", filename)

	// Check file size
	if int64(len(fileContent)) > p.config.MaxFileSize {
		return &models.ProcessingResult{
			Success:   false,
			Message:   fmt.Sprintf("❌ File too large. Maximum size: %d MB", p.config.MaxFileSize/1024/1024),
			ErrorCode: "FILE_TOO_LARGE",
		}
	}

	// Check file extension
	if !strings.HasSuffix(strings.ToLower(filename), ".zip") {
		return &models.ProcessingResult{
			Success:   false,
			Message:   "❌ Only ZIP archives with UPD are supported",
			ErrorCode: "INVALID_FILE_TYPE",
		}
	}

	// Create temporary file
	if err := p.config.EnsureTempDir(); err != nil {
		return &models.ProcessingResult{
			Success:   false,
			Message:   fmt.Sprintf("❌ Failed to create temp directory: %v", err),
			ErrorCode: "TEMP_DIR_ERROR",
		}
	}

	var err error
	tempZipPath, err = p.saveTempFile(fileContent, filename)
	if err != nil {
		return &models.ProcessingResult{
			Success:   false,
			Message:   fmt.Sprintf("❌ Failed to save temp file: %v", err),
			ErrorCode: "TEMP_FILE_ERROR",
		}
	}

	// Parse UPD
	updDocument, err := p.parseUPD(tempZipPath)
	if err != nil {
		p.logger.Errorf("UPD parsing error: %v", err)
		return &models.ProcessingResult{
			Success:   false,
			Message:   fmt.Sprintf("❌ UPD processing error:\n%v", err),
			ErrorCode: "PARSING_ERROR",
		}
	}

	// Upload to MoySkald
	invoiceResult, err := p.uploadToMoySkald(updDocument)
	if err != nil {
		p.logger.Errorf("MoySkald API error: %v", err)
		return &models.ProcessingResult{
			Success:   false,
			Message:   fmt.Sprintf("❌ MoySkald upload error:\n%v", err),
			ErrorCode: "MOYSKLAD_API_ERROR",
		}
	}

	// Create success result
	return p.createSuccessResult(updDocument, invoiceResult)
}

// saveTempFile saves temporary file
func (p *UPDProcessor) saveTempFile(fileContent []byte, filename string) (string, error) {
	tempFile, err := ioutil.TempFile(p.config.TempDir, "upd_*.zip")
	if err != nil {
		return "", err
	}
	defer tempFile.Close()

	if _, err := tempFile.Write(fileContent); err != nil {
		return "", err
	}

	p.logger.Debugf("Temporary file saved: %s", tempFile.Name())
	return tempFile.Name(), nil
}

// parseUPD parses UPD document
func (p *UPDProcessor) parseUPD(zipPath string) (*models.UPDDocument, error) {
	p.logger.Info("Parsing UPD document...")
	return p.parser.ParseUPDArchive(zipPath)
}

// uploadToMoySkald uploads to MoySkald
func (p *UPDProcessor) uploadToMoySkald(updDocument *models.UPDDocument) (map[string]interface{}, error) {
	p.logger.Info("Uploading to MoySkald...")

	// Verify token
	if !p.moyskladAPI.VerifyToken() {
		return nil, fmt.Errorf("invalid MoySkald API token")
	}

	// Create invoice
	return p.moyskladAPI.CreateInvoiceFromUPD(updDocument)
}

// createSuccessResult creates successful processing result
func (p *UPDProcessor) createSuccessResult(updDocument *models.UPDDocument, invoiceResult map[string]interface{}) *models.ProcessingResult {
	// New response structure contains factureout and demand
	factureout, _ := invoiceResult["factureout"].(map[string]interface{})
	demand, _ := invoiceResult["demand"].(map[string]interface{})

	var invoiceID, invoiceName, demandID, demandName string
	if factureout != nil {
		invoiceID, _ = factureout["id"].(string)
		invoiceName, _ = factureout["name"].(string)
	}
	if demand != nil {
		demandID, _ = demand["id"].(string)
		demandName, _ = demand["name"].(string)
	}

	if invoiceName == "" {
		invoiceName = "Не указано"
	}
	if demandName == "" {
		demandName = "Не указано"
	}

	// Get document URLs
	var invoiceURL, demandURL string
	if invoiceID != "" {
		invoiceURL = p.moyskladAPI.GetInvoiceURL(invoiceID)
	}
	if demandID != "" {
		demandURL = p.moyskladAPI.GetDemandURL(demandID)
	}

	// Format detailed message
	message := p.formatSuccessMessage(updDocument, invoiceName, invoiceURL, demandName, demandURL, invoiceResult)

	return &models.ProcessingResult{
		Success:            true,
		Message:            message,
		UPDDocument:        updDocument,
		MoySkladInvoiceID:  invoiceID,
		MoySkladInvoiceURL: invoiceURL,
	}
}

// formatSuccessMessage formats success message
func (p *UPDProcessor) formatSuccessMessage(updDocument *models.UPDDocument, invoiceName, invoiceURL, demandName, demandURL string, invoiceResult map[string]interface{}) string {
	content := updDocument.Content

	message := "✅ UPD successfully processed and uploaded to MoySkald!\n\n"

	// Information about created documents
	message += fmt.Sprintf("📄 Invoice: %s\n", invoiceName)
	message += fmt.Sprintf("📦 Shipment: %s\n", demandName)
	message += fmt.Sprintf(" Date: %s\n\n", content.InvoiceDate.Format("02.01.2006"))

	// Information about participants
	message += fmt.Sprintf("🏢 Supplier: %s", content.Seller.Name)
	if content.Seller.INN != "" {
		message += fmt.Sprintf(" (INN: %s)", content.Seller.INN)
	}
	message += "\n"

	message += fmt.Sprintf("🏪 Buyer: %s", content.Buyer.Name)
	if content.Buyer.INN != "" {
		message += fmt.Sprintf(" (INN: %s)", content.Buyer.INN)
	}
	message += "\n\n"

	// Financial information
	if content.TotalWithVAT.GreaterThan(content.TotalWithoutVAT) {
		message += fmt.Sprintf("💰 Amount without VAT: %s ₽\n", content.TotalWithoutVAT.StringFixed(2))
		message += fmt.Sprintf("🧾 VAT: %s ₽\n", content.TotalVAT.StringFixed(2))
		message += fmt.Sprintf("💵 Total with VAT: %s ₽\n\n", content.TotalWithVAT.StringFixed(2))
	}

	// Links to documents
	message += "🔗 Links in MoySkald:\n"
	if invoiceURL != "" {
		message += fmt.Sprintf("• Invoice: %s\n", invoiceURL)
	}
	if demandURL != "" {
		message += fmt.Sprintf("• Shipment: %s\n", demandURL)
	}

	if updDocument.MetaInfo.DocFlowID != "" {
		message += fmt.Sprintf("\n🆔 Document flow ID: %s", updDocument.MetaInfo.DocFlowID)
	}

	return message
}

// cleanupTempFiles cleans up temporary files
func (p *UPDProcessor) cleanupTempFiles(zipPath string) {
	p.parser.CleanupTempFiles(zipPath)
}

// CheckMoySkaldConnection checks MoySkald connection
func (p *UPDProcessor) CheckMoySkaldConnection() bool {
	return p.moyskladAPI.VerifyToken()
}

// GetMoySkaldStatus gets detailed MoySkald API status
func (p *UPDProcessor) GetMoySkaldStatus() map[string]interface{} {
	return p.moyskladAPI.VerifyAPIAccess()
}