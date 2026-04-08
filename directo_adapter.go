package accounting

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/qbitsoftware/accounting-service/directo"
	"github.com/shopspring/decimal"
)

const directoDateFormat = "2006-01-02"

// directoProvider implements Provider using the Directo API.
type directoProvider struct {
	client *directo.Client
}

func newDirectoProvider(cfg Config) (*directoProvider, error) {
	restAPIKey := ""
	if cfg.Extra != nil {
		restAPIKey = cfg.Extra["rest_api_key"]
	}

	client, err := directo.New(directo.Config{
		Company:    cfg.APIID,
		Token:      cfg.APIKey,
		RestAPIKey: restAPIKey,
		HTTPClient: cfg.HTTPClient,
	})
	if err != nil {
		return nil, fmt.Errorf("directo provider: %w", err)
	}

	return &directoProvider{client: client}, nil
}

func (p *directoProvider) TestConnection(ctx context.Context) error {
	_, err := p.client.ListAccounts(ctx)
	return p.wrapError("TestConnection", err)
}

// --- Invoices ---

func (p *directoProvider) CreateInvoice(ctx context.Context, input CreateInvoiceInput) (*Invoice, error) {
	lines := make([]directo.InvoiceLineXML, len(input.Lines))
	for i, line := range input.Lines {
		lines[i] = directo.InvoiceLineXML{
			ItemCode:    line.Code,
			Description: line.Description,
			Quantity:    line.Quantity.String(),
			Price:       line.UnitPrice.String(),
			TaxCode:     line.TaxID,
			AccountCode: line.AccountCode,
			Unit:        line.UOMName,
			Project:     line.ProjectCode,
			Object:      line.CostCenterCode,
		}
	}

	inv := directo.InvoiceXML{
		Number:       input.InvoiceNo,
		CustomerCode: input.CustomerID,
		CustomerName: input.CustomerName,
		Date:         formatDirectoDate(input.DocDate),
		Deadline:     formatDirectoDate(input.DueDate),
		Currency:     input.Currency,
		RefNo:        input.RefNo,
		Comment:      input.Comment,
		FootComment:  input.FooterComment,
		Confirmed:    "1",
		Lines:        lines,
	}

	_, err := p.client.CreateInvoice(ctx, inv, nil)
	if err != nil {
		return nil, p.wrapError("CreateInvoice", err)
	}

	return &Invoice{
		ID:           input.InvoiceNo,
		Number:       input.InvoiceNo,
		CustomerName: input.CustomerName,
		CustomerID:   input.CustomerID,
		DocDate:      input.DocDate,
		DueDate:      input.DueDate,
		Currency:     input.Currency,
		ReferenceNo:  input.RefNo,
		Status:       InvoiceStatusUnpaid,
	}, nil
}

func (p *directoProvider) GetInvoice(ctx context.Context, id string) (*Invoice, error) {
	inv, err := p.client.GetInvoice(ctx, id)
	if err != nil {
		return nil, p.wrapError("GetInvoice", err)
	}
	mapped := mapDirectoInvoice(*inv)
	return &mapped, nil
}

func (p *directoProvider) GetInvoicePDF(ctx context.Context, id string, deliveryNote bool) (*InvoicePDF, error) {
	return nil, p.wrapError("GetInvoicePDF", fmt.Errorf("not supported by directo API"))
}

func (p *directoProvider) ListInvoices(ctx context.Context, input ListInvoicesInput) ([]Invoice, error) {
	params := directo.InvoiceListParams{}
	if !input.PeriodStart.IsZero() {
		params.DateFrom = formatDirectoDateTime(input.PeriodStart)
	}
	if !input.PeriodEnd.IsZero() {
		params.DateTo = formatDirectoDateTime(input.PeriodEnd)
	}

	items, err := p.client.ListInvoices(ctx, params)
	if err != nil {
		return nil, p.wrapError("ListInvoices", err)
	}

	invoices := make([]Invoice, len(items))
	for i, item := range items {
		invoices[i] = mapDirectoInvoice(item)
	}
	return invoices, nil
}

func (p *directoProvider) DeleteInvoice(ctx context.Context, id string) error {
	_, err := p.client.DeleteInvoice(ctx, id)
	return p.wrapError("DeleteInvoice", err)
}

// --- Customers ---

func (p *directoProvider) CreateCustomer(ctx context.Context, input CreateCustomerInput) (*Customer, error) {
	code := deriveDirectoCustomerCode(input)

	paymentDays := ""
	if input.PaymentDays != nil {
		paymentDays = strconv.Itoa(*input.PaymentDays)
	}

	cust := directo.CustomerXML{
		Code:        code,
		Name:        input.Name,
		RegNo:       input.RegNo,
		VATNo:       input.VATRegNo,
		Email:       input.Email,
		Phone:       input.Phone,
		Address:     input.Address,
		City:        input.City,
		County:      input.County,
		PostalCode:  input.PostalCode,
		Country:     input.CountryCode,
		Currency:    input.Currency,
		Contact:     input.Contact,
		PaymentDays: paymentDays,
	}

	_, err := p.client.CreateCustomer(ctx, cust)
	if err != nil {
		return nil, p.wrapError("CreateCustomer", err)
	}

	return &Customer{
		ID:          code,
		Name:        input.Name,
		RegNo:       input.RegNo,
		VATRegNo:    input.VATRegNo,
		Email:       input.Email,
		Phone:       input.Phone,
		Address:     input.Address,
		City:        input.City,
		County:      input.County,
		PostalCode:  input.PostalCode,
		CountryCode: input.CountryCode,
		Currency:    input.Currency,
		Contact:     input.Contact,
	}, nil
}

func (p *directoProvider) UpdateCustomer(ctx context.Context, input UpdateCustomerInput) error {
	cust := directo.CustomerXML{
		Code: input.ID,
	}
	if input.Name != nil {
		cust.Name = *input.Name
	}
	if input.Email != nil {
		cust.Email = *input.Email
	}
	if input.Phone != nil {
		cust.Phone = *input.Phone
	}
	if input.Address != nil {
		cust.Address = *input.Address
	}
	if input.City != nil {
		cust.City = *input.City
	}
	if input.PostalCode != nil {
		cust.PostalCode = *input.PostalCode
	}
	if input.CountryCode != nil {
		cust.Country = *input.CountryCode
	}
	if input.RegNo != nil {
		cust.RegNo = *input.RegNo
	}
	if input.VATRegNo != nil {
		cust.VATNo = *input.VATRegNo
	}

	_, err := p.client.UpdateCustomer(ctx, cust)
	return p.wrapError("UpdateCustomer", err)
}

func (p *directoProvider) ListCustomers(ctx context.Context, input ListCustomersInput) ([]Customer, error) {
	items, err := p.client.ListCustomers(ctx)
	if err != nil {
		return nil, p.wrapError("ListCustomers", err)
	}

	customers := make([]Customer, len(items))
	for i, item := range items {
		customers[i] = mapDirectoCustomer(item)
	}
	return customers, nil
}

func (p *directoProvider) FindCustomerByEmail(ctx context.Context, email string) (*Customer, error) {
	// Try REST API email filter first
	items, err := p.client.GetCustomerByEmail(ctx, email)
	if err != nil {
		return nil, p.wrapError("FindCustomerByEmail", err)
	}

	email = strings.ToLower(strings.TrimSpace(email))
	for _, item := range items {
		if strings.ToLower(strings.TrimSpace(item.Email)) == email {
			c := mapDirectoCustomer(item)
			return &c, nil
		}
	}

	// If REST filter didn't work, fall back to listing all and filtering client-side
	if len(items) == 0 {
		allItems, err := p.client.ListCustomers(ctx)
		if err != nil {
			return nil, p.wrapError("FindCustomerByEmail", err)
		}
		for _, item := range allItems {
			if strings.ToLower(strings.TrimSpace(item.Email)) == email {
				c := mapDirectoCustomer(item)
				return &c, nil
			}
		}
	}

	return nil, &ProviderError{Provider: "directo", Op: "FindCustomerByEmail", Err: ErrNotFound}
}

// --- Payments ---

func (p *directoProvider) CreatePayment(ctx context.Context, input CreatePaymentInput) error {
	receipt := directo.ReceiptXML{
		CustomerCode: input.CustomerName,
		Date:         formatDirectoDate(input.PaymentDate),
		Currency:     input.Currency,
		BankAccount:  input.BankID,
		Lines: []directo.ReceiptLineXML{
			{
				InvoiceNo: input.InvoiceNo,
				Amount:    input.Amount.String(),
			},
		},
	}

	_, err := p.client.CreatePayment(ctx, receipt)
	return p.wrapError("CreatePayment", err)
}

func (p *directoProvider) ListPayments(ctx context.Context, input ListPaymentsInput) ([]Payment, error) {
	params := directo.PaymentListParams{}
	if !input.PeriodStart.IsZero() {
		params.DateFrom = formatDirectoDateTime(input.PeriodStart)
	}
	if !input.PeriodEnd.IsZero() {
		params.DateTo = formatDirectoDateTime(input.PeriodEnd)
	}

	items, err := p.client.ListPayments(ctx, params)
	if err != nil {
		return nil, p.wrapError("ListPayments", err)
	}

	payments := make([]Payment, len(items))
	for i, item := range items {
		payments[i] = mapDirectoPayment(item)
	}
	return payments, nil
}

func (p *directoProvider) DeletePayment(ctx context.Context, id string) error {
	_, err := p.client.DeletePayment(ctx, id)
	return p.wrapError("DeletePayment", err)
}

// --- Items ---

func (p *directoProvider) CreateItem(ctx context.Context, input CreateItemInput) (*Item, error) {
	item := directo.ItemXML{
		Code:        input.Code,
		Name:        input.Description,
		Description: input.Description,
		Type:        mapItemTypeToDirecto(input.Type),
		Unit:        input.UnitOfMeasure,
		Price:       input.SalesPrice.String(),
		TaxCode:     input.TaxID,
		SalesAcc:    input.SalesAccountCode,
		PurchaseAcc: input.PurchaseAccountCode,
	}

	_, err := p.client.CreateItem(ctx, item)
	if err != nil {
		return nil, p.wrapError("CreateItem", err)
	}

	return &Item{
		ID:            input.Code,
		Code:          input.Code,
		Name:          input.Description,
		Description:   input.Description,
		Type:          input.Type,
		UnitOfMeasure: input.UnitOfMeasure,
		SalesPrice:    input.SalesPrice,
		TaxID:         input.TaxID,
	}, nil
}

func (p *directoProvider) ListItems(ctx context.Context, input ListItemsInput) ([]Item, error) {
	params := directo.ItemListParams{
		Code: input.Code,
	}

	results, err := p.client.ListItems(ctx, params)
	if err != nil {
		return nil, p.wrapError("ListItems", err)
	}

	items := make([]Item, len(results))
	for i, r := range results {
		items[i] = mapDirectoItem(r)
	}
	return items, nil
}

func (p *directoProvider) UpdateItem(ctx context.Context, input UpdateItemInput) error {
	item := directo.ItemXML{
		Code: input.ID,
	}
	if input.Code != nil {
		item.Code = *input.Code
	}
	if input.Description != nil {
		item.Name = *input.Description
		item.Description = *input.Description
	}
	if input.SalesPrice != nil {
		item.Price = input.SalesPrice.String()
	}
	if input.TaxID != nil {
		item.TaxCode = *input.TaxID
	}

	_, err := p.client.UpdateItem(ctx, item)
	return p.wrapError("UpdateItem", err)
}

// --- Credit Notes ---

func (p *directoProvider) CreateCreditNote(ctx context.Context, input CreateCreditNoteInput) (*Invoice, error) {
	// Directo handles credit notes as negative invoices
	lines := make([]directo.InvoiceLineXML, len(input.Lines))
	for i, line := range input.Lines {
		// Negate quantities for credit note
		qty := line.Quantity.Neg()
		lines[i] = directo.InvoiceLineXML{
			ItemCode:    line.Code,
			Description: line.Description,
			Quantity:    qty.String(),
			Price:       line.UnitPrice.String(),
			TaxCode:     line.TaxID,
			AccountCode: line.AccountCode,
			Unit:        line.UOMName,
		}
	}

	inv := directo.InvoiceXML{
		Number:       input.InvoiceNo,
		CustomerCode: input.CustomerID,
		CustomerName: input.CustomerName,
		Date:         formatDirectoDate(input.DocDate),
		Deadline:     formatDirectoDate(input.DueDate),
		Currency:     input.Currency,
		RefNo:        input.RefNo,
		Comment:      input.Comment,
		FootComment:  input.FooterComment,
		Confirmed:    "1",
		Lines:        lines,
	}

	_, err := p.client.CreateInvoice(ctx, inv, nil)
	if err != nil {
		return nil, p.wrapError("CreateCreditNote", err)
	}

	return &Invoice{
		ID:           input.InvoiceNo,
		Number:       input.InvoiceNo,
		CustomerName: input.CustomerName,
		CustomerID:   input.CustomerID,
		DocDate:      input.DocDate,
		DueDate:      input.DueDate,
		Currency:     input.Currency,
		ReferenceNo:  input.RefNo,
		Status:       InvoiceStatusUnpaid,
	}, nil
}

// --- Purchases ---

func (p *directoProvider) CreatePurchase(ctx context.Context, input CreatePurchaseInput) (*PurchaseInvoice, error) {
	return nil, p.wrapError("CreatePurchase", fmt.Errorf("not yet implemented"))
}

func (p *directoProvider) GetPurchase(ctx context.Context, id string) (*PurchaseInvoice, error) {
	return nil, p.wrapError("GetPurchase", fmt.Errorf("not yet implemented"))
}

func (p *directoProvider) ListPurchases(ctx context.Context, input ListPurchasesInput) ([]PurchaseInvoice, error) {
	return nil, p.wrapError("ListPurchases", fmt.Errorf("not yet implemented"))
}

func (p *directoProvider) DeletePurchase(ctx context.Context, id string) error {
	return p.wrapError("DeletePurchase", fmt.Errorf("not yet implemented"))
}

// --- Reference data ---

func (p *directoProvider) ListTaxes(ctx context.Context) ([]Tax, error) {
	items, err := p.client.ListTaxes(ctx)
	if err != nil {
		return nil, p.wrapError("ListTaxes", err)
	}

	taxes := make([]Tax, len(items))
	for i, item := range items {
		pct, _ := decimal.NewFromString(item.Pct)
		taxes[i] = Tax{
			ID:   item.Code,
			Code: item.Code,
			Name: item.Name,
			Pct:  pct,
		}
	}
	return taxes, nil
}

func (p *directoProvider) ListAccounts(ctx context.Context) ([]Account, error) {
	items, err := p.client.ListAccounts(ctx)
	if err != nil {
		return nil, p.wrapError("ListAccounts", err)
	}

	accounts := make([]Account, len(items))
	for i, item := range items {
		accounts[i] = Account{
			ID:     item.Code,
			Code:   item.Code,
			Name:   item.Name,
			Active: item.Status != "closed",
		}
	}
	return accounts, nil
}

func (p *directoProvider) ListDimensions(ctx context.Context) (*DimensionList, error) {
	objects, err := p.client.ListObjects(ctx)
	if err != nil {
		return nil, p.wrapError("ListDimensions", err)
	}

	projects, err := p.client.ListProjects(ctx)
	if err != nil {
		return nil, p.wrapError("ListDimensions", err)
	}

	result := &DimensionList{
		Projects:    make([]Dimension, len(projects)),
		CostCenters: make([]Dimension, len(objects)),
		Departments: nil,
	}

	for i, proj := range projects {
		result.Projects[i] = Dimension{
			Code: proj.Code,
			Name: proj.Name,
		}
	}
	for i, obj := range objects {
		result.CostCenters[i] = Dimension{
			Code: obj.Code,
			Name: obj.Name,
		}
	}

	return result, nil
}

// --- Reports ---

func (p *directoProvider) CustomerDebts(ctx context.Context, customerName string, overdueDays *int) ([]CustomerDebt, error) {
	return nil, p.wrapError("CustomerDebts", fmt.Errorf("not yet implemented"))
}

// --- Sync ---

func (p *directoProvider) ListInvoicesSince(ctx context.Context, since time.Time, until time.Time) ([]Invoice, error) {
	params := directo.InvoiceListParams{
		TSFrom: formatDirectoDateTime(since),
	}

	items, err := p.client.ListInvoices(ctx, params)
	if err != nil {
		return nil, p.wrapError("ListInvoicesSince", err)
	}

	invoices := make([]Invoice, len(items))
	for i, item := range items {
		invoices[i] = mapDirectoInvoice(item)
	}
	return invoices, nil
}

func (p *directoProvider) ListPaymentsSince(ctx context.Context, since time.Time, until time.Time) ([]Payment, error) {
	params := directo.PaymentListParams{
		TSFrom: formatDirectoDateTime(since),
	}

	items, err := p.client.ListPayments(ctx, params)
	if err != nil {
		return nil, p.wrapError("ListPaymentsSince", err)
	}

	payments := make([]Payment, len(items))
	for i, item := range items {
		payments[i] = mapDirectoPayment(item)
	}
	return payments, nil
}

// --- Mapping helpers ---

func mapDirectoInvoice(item directo.InvoiceREST) Invoice {
	total, _ := decimal.NewFromString(item.Total)
	tax, _ := decimal.NewFromString(item.TotalTax)
	paid, _ := decimal.NewFromString(item.PaidAmount)

	return Invoice{
		ID:           item.Number,
		Number:       item.Number,
		CustomerName: item.CustomerName,
		CustomerID:   item.CustomerCode,
		DocDate:      parseDirectoDate(item.Date),
		DueDate:      parseDirectoDate(item.Deadline),
		TotalAmount:  total,
		TaxAmount:    tax,
		PaidAmount:   paid,
		Currency:     item.Currency,
		Paid:         paid.GreaterThanOrEqual(total) && total.IsPositive(),
		Status:       deriveDirectoInvoiceStatus(total, paid),
		ReferenceNo:  item.RefNo,
	}
}

func mapDirectoCustomer(item directo.CustomerREST) Customer {
	return Customer{
		ID:          item.Code,
		Name:        item.Name,
		RegNo:       item.RegNo,
		VATRegNo:    item.VATNo,
		Email:       item.Email,
		Phone:       item.Phone,
		Address:     item.Address,
		City:        item.City,
		County:      item.County,
		PostalCode:  item.PostalCode,
		CountryCode: item.Country,
		Currency:    item.Currency,
		PaymentDays: item.PaymentDays,
		Contact:     item.Contact,
		HomePage:    item.HomePage,
	}
}

func mapDirectoPayment(item directo.ReceiptREST) Payment {
	amount, _ := decimal.NewFromString(item.Amount)

	var links []PaymentInvoiceLink
	if item.InvoiceNo != "" {
		links = []PaymentInvoiceLink{
			{
				InvoiceNo: item.InvoiceNo,
				Amount:    amount,
			},
		}
	}

	return Payment{
		ID:              item.Number,
		DocumentNo:      item.Number,
		DocumentDate:    parseDirectoDate(item.Date),
		Amount:          amount,
		Currency:        item.Currency,
		Direction:       PaymentDirectionCustomer,
		CounterPartID:   item.CustomerCode,
		CounterPartName: item.CustomerName,
		InvoiceLinks:    links,
	}
}

func mapDirectoItem(item directo.ItemREST) Item {
	price, _ := decimal.NewFromString(item.Price)

	return Item{
		ID:            item.Code,
		Code:          item.Code,
		Name:          item.Name,
		Description:   item.Description,
		Type:          mapDirectoItemType(item.Class),
		UnitOfMeasure: item.Unit,
		SalesPrice:    price,
	}
}

func deriveDirectoInvoiceStatus(total, paid decimal.Decimal) InvoiceStatus {
	if total.IsPositive() && paid.GreaterThanOrEqual(total) {
		return InvoiceStatusPaid
	}
	if paid.IsPositive() {
		return InvoiceStatusPartial
	}
	return InvoiceStatusUnpaid
}

// deriveDirectoCustomerCode generates a Directo customer code from input fields.
// Directo requires a unique code for each customer.
func deriveDirectoCustomerCode(input CreateCustomerInput) string {
	// Use RegNo if available (most reliable unique identifier)
	if input.RegNo != "" {
		return input.RegNo
	}
	// Fall back to email prefix
	if input.Email != "" {
		parts := strings.SplitN(input.Email, "@", 2)
		return strings.ToUpper(strings.ReplaceAll(parts[0], ".", "_"))
	}
	// Last resort: sanitize name
	code := strings.ToUpper(input.Name)
	code = strings.ReplaceAll(code, " ", "_")
	if len(code) > 20 {
		code = code[:20]
	}
	return code
}

func mapItemTypeToDirecto(t ItemType) string {
	switch t {
	case ItemTypeService:
		return "0"
	case ItemTypeStock:
		return "1"
	default:
		return "0" // default to service
	}
}

func mapDirectoItemType(class string) ItemType {
	// Directo uses class-based categorization; map common patterns
	lower := strings.ToLower(class)
	switch {
	case strings.Contains(lower, "teenus") || strings.Contains(lower, "service"):
		return ItemTypeService
	case strings.Contains(lower, "kaup") || strings.Contains(lower, "stock") || strings.Contains(lower, "toode"):
		return ItemTypeStock
	default:
		return ItemTypeService
	}
}

func formatDirectoDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(directoDateFormat)
}

func formatDirectoDateTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}

func parseDirectoDate(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	// Try ISO date first
	t, err := time.Parse(directoDateFormat, s)
	if err == nil {
		return t
	}
	// Try RFC3339 / ISO datetime
	t, err = time.Parse(time.RFC3339, s)
	if err == nil {
		return t
	}
	// Try other common formats
	t, err = time.Parse("2006-01-02T15:04:05", s)
	if err == nil {
		return t
	}
	return time.Time{}
}

// wrapError converts Directo APIError status codes into sentinel errors.
func (p *directoProvider) wrapError(op string, err error) error {
	if err == nil {
		return nil
	}

	var apiErr *directo.APIError
	if errors.As(err, &apiErr) {
		var sentinel error
		switch apiErr.StatusCode {
		case 401, 403:
			sentinel = ErrAuthFailed
		case 404:
			sentinel = ErrNotFound
		case 429:
			sentinel = ErrRateLimit
		default:
			sentinel = err
		}
		return &ProviderError{Provider: "directo", Op: op, Err: sentinel}
	}

	return &ProviderError{Provider: "directo", Op: op, Err: err}
}
