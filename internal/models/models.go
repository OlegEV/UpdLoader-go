package models

import (
	"fmt"
	"time"

	"github.com/shopspring/decimal"
)

// Address represents an organization address
type Address struct {
	PostalCode string `json:"postal_code,omitempty"`
	RegionCode string `json:"region_code,omitempty"`
	Region     string `json:"region,omitempty"`
	City       string `json:"city,omitempty"`
	Street     string `json:"street,omitempty"`
	House      string `json:"house,omitempty"`
	Apartment  string `json:"apartment,omitempty"`
}

// MetaInfo contains metadata from meta.xml
type MetaInfo struct {
	DocFlowID        string `json:"doc_flow_id"`
	MainDocumentPath string `json:"main_document_path"`
	CardPath         string `json:"card_path"`
}

// CardInfo contains information from card.xml
type CardInfo struct {
	ExternalIdentifier string    `json:"external_identifier"`
	Title              string    `json:"title"`
	Date               time.Time `json:"date"`
	SenderINN          string    `json:"sender_inn,omitempty"`
	SenderKPP          string    `json:"sender_kpp,omitempty"`
	SenderName         string    `json:"sender_name,omitempty"`
}

// InvoiceItem represents an invoice line item
type InvoiceItem struct {
	LineNumber       int             `json:"line_number"`
	Name             string          `json:"name"`
	UnitCode         string          `json:"unit_code,omitempty"`
	UnitName         string          `json:"unit_name,omitempty"`
	Quantity         decimal.Decimal `json:"quantity"`
	Price            decimal.Decimal `json:"price"`
	AmountWithoutVAT decimal.Decimal `json:"amount_without_vat"`
	VATRate          string          `json:"vat_rate,omitempty"`
	VATAmount        decimal.Decimal `json:"vat_amount"`
	AmountWithVAT    decimal.Decimal `json:"amount_with_vat"`
	Article          string          `json:"article,omitempty"`
}

// Organization represents organization information
type Organization struct {
	Name    string   `json:"name"`
	INN     string   `json:"inn"`
	KPP     string   `json:"kpp,omitempty"`
	Address *Address `json:"address,omitempty"`
}

// UPDContent represents the main UPD content
type UPDContent struct {
	// Invoice information
	InvoiceNumber string    `json:"invoice_number"`
	InvoiceDate   time.Time `json:"invoice_date"`
	Seller        Organization `json:"seller"`
	Buyer         Organization `json:"buyer"`

	// Optional fields with defaults
	Items           []InvoiceItem   `json:"items"`
	CurrencyCode    string          `json:"currency_code"`
	TotalWithoutVAT decimal.Decimal `json:"total_without_vat"`
	TotalVAT        decimal.Decimal `json:"total_vat"`
	TotalWithVAT    decimal.Decimal `json:"total_with_vat"`
	RequisiteNumber string          `json:"requisite_number,omitempty"`
}

// UPDDocument represents a complete UPD document
type UPDDocument struct {
	MetaInfo MetaInfo   `json:"meta_info"`
	CardInfo CardInfo   `json:"card_info"`
	Content  UPDContent `json:"content"`
}

// DocumentID returns the unique document identifier
func (u *UPDDocument) DocumentID() string {
	return u.CardInfo.ExternalIdentifier
}

// Summary returns a brief description of the document
func (u *UPDDocument) Summary() string {
	return fmt.Sprintf(
		"УПД № %s от %s\nПоставщик: %s (ИНН: %s)\nПокупатель: %s (ИНН: %s)\nСумма: %s ₽",
		u.Content.InvoiceNumber,
		u.Content.InvoiceDate.Format("02.01.2006"),
		u.Content.Seller.Name,
		u.Content.Seller.INN,
		u.Content.Buyer.Name,
		u.Content.Buyer.INN,
		u.Content.TotalWithVAT.StringFixed(2),
	)
}

// ProcessingResult represents the result of UPD processing
type ProcessingResult struct {
	Success              bool        `json:"success"`
	Message              string      `json:"message"`
	UPDDocument          *UPDDocument `json:"upd_document,omitempty"`
	MoySkladInvoiceID    string      `json:"moysklad_invoice_id,omitempty"`
	MoySkladInvoiceURL   string      `json:"moysklad_invoice_url,omitempty"`
	ErrorCode            string      `json:"error_code,omitempty"`
}

// NewUPDContent creates a new UPDContent with default values
func NewUPDContent(invoiceNumber string, invoiceDate time.Time, seller, buyer Organization) *UPDContent {
	return &UPDContent{
		InvoiceNumber:   invoiceNumber,
		InvoiceDate:     invoiceDate,
		Seller:          seller,
		Buyer:           buyer,
		Items:           make([]InvoiceItem, 0),
		CurrencyCode:    "643", // RUB
		TotalWithoutVAT: decimal.Zero,
		TotalVAT:        decimal.Zero,
		TotalWithVAT:    decimal.Zero,
	}
}