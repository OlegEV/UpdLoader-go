package parser

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"github.com/sirupsen/logrus"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/transform"

	"upd-loader-go/internal/models"
)

// UPDParsingError represents a UPD parsing error
type UPDParsingError struct {
	Message string
}

func (e *UPDParsingError) Error() string {
	return e.Message
}

// UPDParser handles UPD document parsing
type UPDParser struct {
	encoding string
	logger   *logrus.Logger
}

// NewUPDParser creates a new UPD parser
func NewUPDParser(encoding string, logger *logrus.Logger) *UPDParser {
	return &UPDParser{
		encoding: encoding,
		logger:   logger,
	}
}

// ParseUPDArchive parses a UPD archive
func (p *UPDParser) ParseUPDArchive(zipPath string) (*models.UPDDocument, error) {
	p.logger.Infof("Starting UPD archive parsing: %s", zipPath)

	// Extract archive
	extractDir, err := p.extractArchive(zipPath)
	if err != nil {
		return nil, &UPDParsingError{Message: fmt.Sprintf("Error extracting archive: %v", err)}
	}
	defer p.cleanupExtractDir(extractDir)

	// Parse meta.xml
	metaInfo, err := p.parseMetaXML(extractDir)
	if err != nil {
		return nil, &UPDParsingError{Message: fmt.Sprintf("Error parsing meta.xml: %v", err)}
	}

	// Parse card.xml
	cardInfo, err := p.parseCardXML(extractDir, metaInfo.CardPath)
	if err != nil {
		return nil, &UPDParsingError{Message: fmt.Sprintf("Error parsing card.xml: %v", err)}
	}

	// Parse main UPD document
	content, err := p.parseUPDContent(extractDir, metaInfo.MainDocumentPath)
	if err != nil {
		return nil, &UPDParsingError{Message: fmt.Sprintf("Error parsing UPD content: %v", err)}
	}

	updDocument := &models.UPDDocument{
		MetaInfo: *metaInfo,
		CardInfo: *cardInfo,
		Content:  *content,
	}

	p.logger.Infof("UPD successfully parsed: %s", updDocument.DocumentID())
	return updDocument, nil
}

// extractArchive extracts ZIP archive to temporary directory
func (p *UPDParser) extractArchive(zipPath string) (string, error) {
	extractDir := filepath.Join(filepath.Dir(zipPath), "upd_extract")

	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return "", fmt.Errorf("invalid ZIP file: %v", err)
	}
	defer reader.Close()

	// Create extract directory
	if err := os.MkdirAll(extractDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create extract directory: %v", err)
	}

	// Extract files
	for _, file := range reader.File {
		path := filepath.Join(extractDir, file.Name)

		// Security check
		if !strings.HasPrefix(path, filepath.Clean(extractDir)+string(os.PathSeparator)) {
			return "", fmt.Errorf("invalid file path: %s", file.Name)
		}

		if file.FileInfo().IsDir() {
			os.MkdirAll(path, file.FileInfo().Mode())
			continue
		}

		// Create file
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return "", err
		}

		fileReader, err := file.Open()
		if err != nil {
			return "", err
		}

		targetFile, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, file.FileInfo().Mode())
		if err != nil {
			fileReader.Close()
			return "", err
		}

		_, err = io.Copy(targetFile, fileReader)
		fileReader.Close()
		targetFile.Close()

		if err != nil {
			return "", err
		}
	}

	p.logger.Debugf("Archive extracted to: %s", extractDir)
	return extractDir, nil
}

// parseMetaXML parses meta.xml file
func (p *UPDParser) parseMetaXML(extractDir string) (*models.MetaInfo, error) {
	metaPath := filepath.Join(extractDir, "meta.xml")

	if _, err := os.Stat(metaPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("meta.xml not found in archive")
	}

	data, err := os.ReadFile(metaPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read meta.xml: %v", err)
	}

	// Parse XML structure for meta.xml
	type MetaXML struct {
		XMLName  xml.Name `xml:"DocumentPackage"`
		DocFlows []struct {
			ID         string `xml:"Id,attr"`
			MainImage  struct {
				Path string `xml:"Path,attr"`
			} `xml:"MainImage"`
			ExternalCard struct {
				Path string `xml:"Path,attr"`
			} `xml:"ExternalCard"`
		} `xml:"DocFlow"`
	}

	var meta MetaXML
	if err := xml.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("failed to parse meta.xml: %v", err)
	}

	if len(meta.DocFlows) == 0 {
		return nil, fmt.Errorf("no DocFlow found in meta.xml")
	}

	docFlow := meta.DocFlows[0]
	if docFlow.ID == "" {
		return nil, fmt.Errorf("DocFlow ID not found")
	}

	if docFlow.MainImage.Path == "" || docFlow.ExternalCard.Path == "" {
		return nil, fmt.Errorf("file paths not found in meta.xml")
	}

	return &models.MetaInfo{
		DocFlowID:        docFlow.ID,
		MainDocumentPath: docFlow.MainImage.Path,
		CardPath:         docFlow.ExternalCard.Path,
	}, nil
}

// parseCardXML parses card.xml file
func (p *UPDParser) parseCardXML(extractDir, cardPath string) (*models.CardInfo, error) {
	fullCardPath := filepath.Join(extractDir, cardPath)

	if _, err := os.Stat(fullCardPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("card.xml not found: %s", cardPath)
	}

	content, err := p.readFileWithEncoding(fullCardPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read card.xml: %v", err)
	}

	// Parse XML structure for card.xml
	type CardXML struct {
		XMLName     xml.Name `xml:"Card"`
		Identifiers struct {
			ExternalIdentifier string `xml:"ExternalIdentifier,attr"`
		} `xml:"Identifiers"`
		Description struct {
			Title string `xml:"Title,attr"`
			Date  string `xml:"Date,attr"`
		} `xml:"Description"`
		Sender struct {
			Abonent struct {
				INN  string `xml:"Inn,attr"`
				KPP  string `xml:"Kpp,attr"`
				Name string `xml:"Name,attr"`
			} `xml:"Abonent"`
		} `xml:"Sender"`
	}

	var card CardXML
	if err := xml.Unmarshal([]byte(content), &card); err != nil {
		return nil, fmt.Errorf("failed to parse card.xml: %v", err)
	}

	// Parse date
	date := time.Now()
	if card.Description.Date != "" {
		if parsedDate, err := time.Parse(time.RFC3339, strings.Replace(card.Description.Date, "Z", "+00:00", 1)); err == nil {
			date = parsedDate
		}
	}

	return &models.CardInfo{
		ExternalIdentifier: card.Identifiers.ExternalIdentifier,
		Title:              card.Description.Title,
		Date:               date,
		SenderINN:          card.Sender.Abonent.INN,
		SenderKPP:          card.Sender.Abonent.KPP,
		SenderName:         card.Sender.Abonent.Name,
	}, nil
}

// parseUPDContent parses the main UPD document
func (p *UPDParser) parseUPDContent(extractDir, mainDocumentPath string) (*models.UPDContent, error) {
	fullUPDPath := filepath.Join(extractDir, mainDocumentPath)

	if _, err := os.Stat(fullUPDPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("main UPD file not found: %s", mainDocumentPath)
	}

	content, err := p.readFileWithEncoding(fullUPDPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read UPD file: %v", err)
	}

	// If file contains only XML header, create basic structure
	if len(strings.TrimSpace(content)) <= 100 {
		p.logger.Warning("UPD file contains only XML header, creating basic structure")
		return p.createBasicUPDContent(), nil
	}

	// Parse full UPD content
	return p.parseFullUPDContent(content)
}

// createBasicUPDContent creates basic UPD structure when full data is not available
func (p *UPDParser) createBasicUPDContent() *models.UPDContent {
	return models.NewUPDContent(
		"Не указан",
		time.Now(),
		models.Organization{Name: "Не указано", INN: "0000000000"},
		models.Organization{Name: "Не указано", INN: "0000000000"},
	)
}

// parseFullUPDContent parses full UPD content from XML
func (p *UPDParser) parseFullUPDContent(content string) (*models.UPDContent, error) {
	p.logger.Info("Parsing full UPD document...")

	// Define XML structure for UPD 5.03
	type UPDXML struct {
		XMLName    xml.Name `xml:"Файл"`
		Version    string   `xml:"ВерсФорм,attr"`
		InvoiceInfo struct {
			Number string `xml:"НомерДок,attr"`
			Date   string `xml:"ДатаДок,attr"`
		} `xml:"СвСчФакт"`
		SellerInfo struct {
			LegalEntity struct {
				Name string `xml:"НаимОрг,attr"`
				INN  string `xml:"ИННЮЛ,attr"`
				KPP  string `xml:"КПП,attr"`
			} `xml:"ИдСв>СвЮЛУч"`
			Individual struct {
				INN string `xml:"ИННФЛ,attr"`
				FIO struct {
					Surname    string `xml:"Фамилия,attr"`
					Name       string `xml:"Имя,attr"`
					Patronymic string `xml:"Отчество,attr"`
				} `xml:"ФИО"`
			} `xml:"ИдСв>СвИП"`
		} `xml:"СвПрод"`
		BuyerInfo struct {
			LegalEntity struct {
				Name string `xml:"НаимОрг,attr"`
				INN  string `xml:"ИННЮЛ,attr"`
				KPP  string `xml:"КПП,attr"`
			} `xml:"ИдСв>СвЮЛУч"`
			Individual struct {
				INN string `xml:"ИННФЛ,attr"`
				FIO struct {
					Surname    string `xml:"Фамилия,attr"`
					Name       string `xml:"Имя,attr"`
					Patronymic string `xml:"Отчество,attr"`
				} `xml:"ФИО"`
			} `xml:"ИдСв>СвИП"`
		} `xml:"ГрузПолуч"`
		Table struct {
			Items []struct {
				Name           string `xml:"НаимТов,attr"`
				Quantity       string `xml:"КолТов,attr"`
				Price          string `xml:"ЦенаТов,attr"`
				AmountWithoutVAT string `xml:"СтТовБезНДС,attr"`
				VATRate        string `xml:"НалСт,attr"`
				AmountWithVAT  string `xml:"СтТовУчНал,attr"`
				Additional struct {
					Article string `xml:"КодТов,attr"`
				} `xml:"ДопСведТов"`
				VATSum struct {
					Amount string `xml:",chardata"`
				} `xml:"СумНал>СумНал"`
			} `xml:"СведТов"`
		} `xml:"ТаблСчФакт"`
		Totals struct {
			TotalWithoutVAT string `xml:"СтТовБезНДСВсего"`
			TotalWithVAT    string `xml:"СтТовУчНалВсего"`
			VATSum          string `xml:"СумНал"`
		} `xml:"ВсегоОпл"`
		Transfer struct {
			Basis struct {
				RequisiteNumber string `xml:"РеквНомерДок,attr"`
			} `xml:"СвПер>ОснПер"`
		} `xml:"СвПродПер"`
	}

	var upd UPDXML
	if err := xml.Unmarshal([]byte(content), &upd); err != nil {
		p.logger.Warningf("Error parsing full UPD: %v, creating basic structure", err)
		return p.createBasicUPDContent(), nil
	}

	// Parse invoice number and date
	invoiceNumber := upd.InvoiceInfo.Number
	if invoiceNumber == "" {
		invoiceNumber = "Не указан"
	}

	invoiceDate := time.Now()
	if upd.InvoiceInfo.Date != "" {
		if parsedDate, err := time.Parse("02.01.2006", upd.InvoiceInfo.Date); err == nil {
			invoiceDate = parsedDate
		}
	}

	// Parse seller
	seller := p.parseOrganization(upd.SellerInfo.LegalEntity.Name, upd.SellerInfo.LegalEntity.INN, upd.SellerInfo.LegalEntity.KPP,
		upd.SellerInfo.Individual.INN, upd.SellerInfo.Individual.FIO.Surname, upd.SellerInfo.Individual.FIO.Name, upd.SellerInfo.Individual.FIO.Patronymic)

	// Parse buyer
	buyer := p.parseOrganization(upd.BuyerInfo.LegalEntity.Name, upd.BuyerInfo.LegalEntity.INN, upd.BuyerInfo.LegalEntity.KPP,
		upd.BuyerInfo.Individual.INN, upd.BuyerInfo.Individual.FIO.Surname, upd.BuyerInfo.Individual.FIO.Name, upd.BuyerInfo.Individual.FIO.Patronymic)

	// Parse items
	items := p.parseInvoiceItems(upd.Table.Items)

	// Parse totals
	totalWithoutVAT := p.parseDecimal(upd.Totals.TotalWithoutVAT)
	totalWithVAT := p.parseDecimal(upd.Totals.TotalWithVAT)
	totalVAT := p.parseDecimal(upd.Totals.VATSum)

	// Extract requisite number
	requisiteNumber := ""
	if upd.Transfer.Basis.RequisiteNumber != "" {
		// Extract only numbers from requisite
		re := regexp.MustCompile(`\d+`)
		numbers := re.FindAllString(upd.Transfer.Basis.RequisiteNumber, -1)
		if len(numbers) > 0 {
			requisiteNumber = numbers[0]
		}
	}

	updContent := models.NewUPDContent(invoiceNumber, invoiceDate, seller, buyer)
	updContent.Items = items
	updContent.TotalWithoutVAT = totalWithoutVAT
	updContent.TotalVAT = totalVAT
	updContent.TotalWithVAT = totalWithVAT
	updContent.RequisiteNumber = requisiteNumber

	p.logger.Infof("UPD parsed: № %s, seller INN %s, buyer INN %s", invoiceNumber, seller.INN, buyer.INN)

	return updContent, nil
}

// parseOrganization parses organization from legal entity or individual data
func (p *UPDParser) parseOrganization(legalName, legalINN, legalKPP, individualINN, surname, name, patronymic string) models.Organization {
	// Try legal entity first
	if legalINN != "" {
		return models.Organization{
			Name: legalName,
			INN:  legalINN,
			KPP:  legalKPP,
		}
	}

	// Try individual
	if individualINN != "" {
		fullName := strings.TrimSpace(fmt.Sprintf("%s %s %s", surname, name, patronymic))
		if fullName == "" {
			fullName = "Не указано"
		}
		return models.Organization{
			Name: fullName,
			INN:  individualINN,
		}
	}

	// Default
	return models.Organization{
		Name: "Не указано",
		INN:  "0000000000",
	}
}

// parseInvoiceItems parses invoice items from XML
func (p *UPDParser) parseInvoiceItems(xmlItems []struct {
	Name           string `xml:"НаимТов,attr"`
	Quantity       string `xml:"КолТов,attr"`
	Price          string `xml:"ЦенаТов,attr"`
	AmountWithoutVAT string `xml:"СтТовБезНДС,attr"`
	VATRate        string `xml:"НалСт,attr"`
	AmountWithVAT  string `xml:"СтТовУчНал,attr"`
	Additional struct {
		Article string `xml:"КодТов,attr"`
	} `xml:"ДопСведТов"`
	VATSum struct {
		Amount string `xml:",chardata"`
	} `xml:"СумНал>СумНал"`
}) []models.InvoiceItem {
	var items []models.InvoiceItem

	for i, xmlItem := range xmlItems {
		item := models.InvoiceItem{
			LineNumber:       i + 1,
			Name:             xmlItem.Name,
			Quantity:         p.parseDecimal(xmlItem.Quantity),
			Price:            p.parseDecimal(xmlItem.Price),
			AmountWithoutVAT: p.parseDecimal(xmlItem.AmountWithVAT), // Use amount with VAT as main amount
			VATRate:          xmlItem.VATRate,
			VATAmount:        p.parseDecimal(xmlItem.VATSum.Amount),
			AmountWithVAT:    p.parseDecimal(xmlItem.AmountWithVAT),
			Article:          xmlItem.Additional.Article,
		}

		items = append(items, item)
		p.logger.Debugf("Item %d: %s, article: %s, quantity: %s, price: %s, amount with VAT: %s",
			i+1, item.Name, item.Article, item.Quantity, item.Price, item.AmountWithVAT)
	}

	p.logger.Infof("Parsed %d items", len(items))
	return items
}

// parseDecimal safely parses decimal from string
func (p *UPDParser) parseDecimal(s string) decimal.Decimal {
	if s == "" {
		return decimal.Zero
	}

	if d, err := decimal.NewFromString(s); err == nil {
		return d
	}

	return decimal.Zero
}

// readFileWithEncoding reads file with specified encoding
func (p *UPDParser) readFileWithEncoding(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	// Use Windows-1251 decoder
	decoder := charmap.Windows1251.NewDecoder()
	reader := transform.NewReader(file, decoder)

	content, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}

	return string(content), nil
}

// cleanupExtractDir removes the extraction directory
func (p *UPDParser) cleanupExtractDir(extractDir string) {
	if err := os.RemoveAll(extractDir); err != nil {
		p.logger.Errorf("Failed to cleanup extract directory: %v", err)
	}
}

// CleanupTempFiles removes temporary files
func (p *UPDParser) CleanupTempFiles(zipPath string) {
	// Remove original ZIP file
	if err := os.Remove(zipPath); err != nil {
		p.logger.Errorf("Failed to remove ZIP file: %v", err)
	}

	// Remove extract directory
	extractDir := filepath.Join(filepath.Dir(zipPath), "upd_extract")
	if err := os.RemoveAll(extractDir); err != nil {
		p.logger.Errorf("Failed to remove extract directory: %v", err)
	}

	p.logger.Debug("Temporary files cleaned up")
}