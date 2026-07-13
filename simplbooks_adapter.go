package accounting

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/qbitsoftware/accounting-service/simplbooks"
)

const simplbooksDateFormat = "2006-01-02"

// simplbooksProvider implements Provider using the SimplBooks API.
//
// SimplBooks identifies every object by a numeric ID (client_id, article_id,
// vat_type_id, income_account_id, ...) rather than by a caller-assigned code, so
// the ID fields on the returned Customer/Item/Invoice carry the SimplBooks
// numeric ID as a string. Callers that reference an object on create — e.g.
// CreateInvoice.CustomerID, line.TaxID / AccountCode, CreatePayment.BankID —
// must pass that numeric ID as a string; the adapter parses it back to an int.
type simplbooksProvider struct {
	client *simplbooks.Client
}

func newSimplBooksProvider(cfg Config) *simplbooksProvider {
	// CompanyID: prefer APIID, fall back to Extra["company_id"]. Token: APIKey.
	companyID := cfg.APIID
	if companyID == "" {
		companyID = cfg.Extra["company_id"]
	}
	baseURL := strings.TrimRight(cfg.Extra["base_url"], "/")
	if baseURL != "" && !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
		baseURL = "https://" + baseURL
	}
	return &simplbooksProvider{
		client: simplbooks.New(simplbooks.Config{
			BaseURL:    baseURL,
			CompanyID:  companyID,
			Token:      cfg.APIKey,
			HTTPClient: cfg.HTTPClient,
		}),
	}
}

func (p *simplbooksProvider) TestConnection(ctx context.Context) error {
	_, err := p.client.ListVatTypes(ctx)
	return p.wrapError("TestConnection", err)
}

// --- Invoices ---

func (p *simplbooksProvider) CreateInvoice(ctx context.Context, input CreateInvoiceInput) (*Invoice, error) {
	invoice := map[string]any{
		"created":          formatSimplBooksDate(input.DocDate),
		"transaction_date": formatSimplBooksDate(input.DocDate),
		"due":              formatSimplBooksDate(input.DueDate),
		"row_sum_with_vat": false, // line prices are net; VAT is added per row
	}
	if id := atoiOrZero(input.CustomerID); id != 0 {
		invoice["client_id"] = id
	}
	if input.Currency != "" {
		invoice["currency_name"] = input.Currency
	}
	if input.RefNo != "" {
		invoice["reference"] = input.RefNo
	}
	if input.InvoiceNo != "" {
		invoice["number"] = input.InvoiceNo
	}
	if input.Comment != "" {
		invoice["additional_info"] = input.Comment
	}

	tasks := make([]map[string]any, 0, len(input.Lines))
	for _, line := range input.Lines {
		tasks = append(tasks, map[string]any{"Task": simplbooksRow(line)})
	}

	id, err := p.client.CreateInvoice(ctx, map[string]any{
		"Invoice": invoice,
		"Tasks":   tasks,
	})
	if err != nil {
		return nil, p.wrapError("CreateInvoice", err)
	}
	return p.GetInvoice(ctx, strconv.Itoa(id))
}

func (p *simplbooksProvider) GetInvoice(ctx context.Context, id string) (*Invoice, error) {
	inv, err := p.client.GetInvoice(ctx, id)
	if err != nil {
		return nil, p.wrapError("GetInvoice", err)
	}
	return mapSimplBooksInvoice(inv), nil
}

func (p *simplbooksProvider) GetInvoicePDF(ctx context.Context, id string, _ bool) (*InvoicePDF, error) {
	content, err := p.client.GetInvoicePDF(ctx, id)
	if err != nil {
		return nil, p.wrapError("GetInvoicePDF", err)
	}
	return &InvoicePDF{FileName: "invoice-" + id + ".pdf", FileContent: content}, nil
}

func (p *simplbooksProvider) ListInvoices(ctx context.Context, input ListInvoicesInput) ([]Invoice, error) {
	filter := map[string]any{}
	if code := atoiOrZero(input.CustomerCode); code != 0 {
		filter["client_id"] = code
	}
	items, err := p.client.ListAllInvoices(ctx, filter, nil, 200)
	if err != nil {
		return nil, p.wrapError("ListInvoices", err)
	}
	// SimplBooks' list date filter uses an ambiguous dd-mm-yyyy format, so we
	// filter the (YYYY-MM-DD) creation date client-side for correctness.
	invoices := make([]Invoice, 0, len(items))
	for i := range items {
		created := parseSimplBooksDate(items[i].Created)
		if !inDateRange(created, input.PeriodStart, input.PeriodEnd) {
			continue
		}
		invoices = append(invoices, *mapSimplBooksInvoice(&items[i]))
	}
	return invoices, nil
}

func (p *simplbooksProvider) DeleteInvoice(ctx context.Context, id string) error {
	return p.wrapError("DeleteInvoice", p.client.DeleteInvoice(ctx, id))
}

// FindInvoiceByRef scans invoices for a matching reference. SimplBooks' list
// endpoint has no reference filter, so we request the reference column via the
// `fields` param and match client-side across pages.
func (p *simplbooksProvider) FindInvoiceByRef(ctx context.Context, refStr string) (*Invoice, error) {
	refStr = strings.TrimSpace(refStr)
	if refStr == "" {
		return nil, &ProviderError{Provider: "simplbooks", Op: "FindInvoiceByRef", Err: ErrNotFound}
	}
	items, err := p.client.ListAllInvoices(ctx, nil, []string{"id", "number", "reference"}, 200)
	if err != nil {
		return nil, p.wrapError("FindInvoiceByRef", err)
	}
	for i := range items {
		if strings.TrimSpace(items[i].Reference) == refStr {
			// The list projection is partial; fetch the full document.
			return p.GetInvoice(ctx, strconv.Itoa(items[i].ID))
		}
	}
	return nil, &ProviderError{Provider: "simplbooks", Op: "FindInvoiceByRef", Err: ErrNotFound}
}

// --- Customers ---

func (p *simplbooksProvider) CreateCustomer(ctx context.Context, input CreateCustomerInput) (*Customer, error) {
	fields := map[string]any{"name": input.Name}
	putIf(fields, "reg_no", input.RegNo)
	putIf(fields, "vat_no", input.VATRegNo)
	putIf(fields, "e_mail", input.Email)
	putIf(fields, "phone", input.Phone)
	putIf(fields, "address_street", input.Address)
	putIf(fields, "address_city", input.City)
	putIf(fields, "address_postal_code", input.PostalCode)
	putIf(fields, "address_county", input.County)
	putIf(fields, "address_country", input.CountryCode)

	id, err := p.client.CreateClient(ctx, fields)
	if err != nil {
		return nil, p.wrapError("CreateCustomer", err)
	}
	cust, err := p.client.GetClient(ctx, strconv.Itoa(id))
	if err != nil {
		// Creation succeeded; return the minimal record rather than failing.
		return &Customer{ID: strconv.Itoa(id), Name: input.Name}, nil
	}
	return mapSimplBooksCustomer(cust), nil
}

func (p *simplbooksProvider) UpdateCustomer(ctx context.Context, input UpdateCustomerInput) error {
	id := atoiOrZero(input.ID)
	if id == 0 {
		return &ProviderError{Provider: "simplbooks", Op: "UpdateCustomer", Err: ErrInvalidInput}
	}
	fields := map[string]any{}
	putIfPtr(fields, "name", input.Name)
	putIfPtr(fields, "e_mail", input.Email)
	putIfPtr(fields, "phone", input.Phone)
	putIfPtr(fields, "address_street", input.Address)
	putIfPtr(fields, "address_city", input.City)
	putIfPtr(fields, "address_postal_code", input.PostalCode)
	putIfPtr(fields, "address_country", input.CountryCode)
	putIfPtr(fields, "reg_no", input.RegNo)
	putIfPtr(fields, "vat_no", input.VATRegNo)

	return p.wrapError("UpdateCustomer", p.client.UpdateClient(ctx, id, fields))
}

func (p *simplbooksProvider) ListCustomers(ctx context.Context, _ ListCustomersInput) ([]Customer, error) {
	items, err := p.client.ListAllClients(ctx, nil, 200)
	if err != nil {
		return nil, p.wrapError("ListCustomers", err)
	}
	customers := make([]Customer, len(items))
	for i := range items {
		customers[i] = *mapSimplBooksCustomer(&items[i])
	}
	return customers, nil
}

func (p *simplbooksProvider) GetCustomer(ctx context.Context, id string) (*Customer, error) {
	if strings.TrimSpace(id) == "" {
		return nil, &ProviderError{Provider: "simplbooks", Op: "GetCustomer", Err: ErrNotFound}
	}
	cust, err := p.client.GetClient(ctx, id)
	if err != nil {
		return nil, p.wrapError("GetCustomer", err)
	}
	return mapSimplBooksCustomer(cust), nil
}

// FindCustomerByEmail searches by email (server-side "contains") and confirms an
// exact, case-insensitive match. An empty email never matches.
func (p *simplbooksProvider) FindCustomerByEmail(ctx context.Context, email string) (*Customer, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return nil, &ProviderError{Provider: "simplbooks", Op: "FindCustomerByEmail", Err: ErrNotFound}
	}
	items, err := p.client.ListAllClients(ctx, map[string]any{"e_mail": email}, 200)
	if err != nil {
		return nil, p.wrapError("FindCustomerByEmail", err)
	}
	for i := range items {
		if strings.ToLower(strings.TrimSpace(items[i].Email)) == email {
			return mapSimplBooksCustomer(&items[i]), nil
		}
	}
	return nil, &ProviderError{Provider: "simplbooks", Op: "FindCustomerByEmail", Err: ErrNotFound}
}

// --- Payments ---

// CreatePayment records a customer receipt (Incoming) bound to an invoice. The
// invoice is settled when the sum covers its balance.
func (p *simplbooksProvider) CreatePayment(ctx context.Context, input CreatePaymentInput) error {
	bankID := atoiOrZero(input.BankID)
	if bankID == 0 {
		return p.wrapError("CreatePayment", fmt.Errorf("BankID (income_account_id) is required for SimplBooks receipts"))
	}
	fields := map[string]any{
		"income_account_id": bankID,
		"income_date":       formatSimplBooksDate(input.PaymentDate),
	}
	if !input.Amount.IsZero() {
		fields["income_sum"] = input.Amount.String()
	}
	if input.Currency != "" {
		fields["currency_name"] = input.Currency
	}
	if code := atoiOrZero(input.CustomerCode); code != 0 {
		fields["client_id"] = code
	}
	if input.InvoiceNo != "" {
		fields["description"] = "Invoice " + input.InvoiceNo
	}
	// InvoiceNo carries the SimplBooks invoice ID for the bound receipt.
	_, err := p.client.CreateIncoming(ctx, fields, atoiOrZero(input.InvoiceNo))
	return p.wrapError("CreatePayment", err)
}

func (p *simplbooksProvider) ListPayments(ctx context.Context, input ListPaymentsInput) ([]Payment, error) {
	items, err := p.client.ListAllIncomings(ctx, nil, 200)
	if err != nil {
		return nil, p.wrapError("ListPayments", err)
	}
	payments := make([]Payment, 0, len(items))
	for i := range items {
		date := parseSimplBooksDate(items[i].IncomeDate)
		if !inDateRange(date, input.PeriodStart, input.PeriodEnd) {
			continue
		}
		// NOTE: incomings/list does not expose the bound invoice, so
		// InvoiceLinks is empty. Reconcile invoice settlement via the invoice's
		// `paid`/`remnant` fields (ListInvoices) rather than payment links.
		payments = append(payments, Payment{
			ID:              strconv.Itoa(items[i].ID),
			DocumentNo:      strconv.Itoa(items[i].ID),
			DocumentDate:    date,
			Amount:          items[i].IncomeSum.Decimal,
			Currency:        items[i].CurrencyName,
			Direction:       PaymentDirectionCustomer,
			CounterPartID:   strconv.Itoa(items[i].ClientID),
			ExternalPayMode: strconv.Itoa(items[i].IncomeAccountID),
		})
	}
	return payments, nil
}

func (p *simplbooksProvider) DeletePayment(ctx context.Context, id string) error {
	return p.wrapError("DeletePayment", p.client.DeleteIncoming(ctx, id))
}

// --- Items ---

func (p *simplbooksProvider) CreateItem(ctx context.Context, input CreateItemInput) (*Item, error) {
	fields := map[string]any{
		"code":   input.Code,
		"name":   input.Description,
		"active": true,
	}
	if input.UnitOfMeasure != "" {
		fields["unit"] = input.UnitOfMeasure
	}
	if !input.SalesPrice.IsZero() {
		fields["price_per_unit"] = input.SalesPrice.String()
	}
	fields["is_inventory"] = input.Type == ItemTypeStock
	// NOTE: SimplBooks' articles/create does not accept a VAT type or sales
	// account (both are read-only on the Article schema). TaxID and
	// SalesAccountCode are set at invoice-line time instead.

	id, err := p.client.CreateArticle(ctx, fields)
	if err != nil {
		return nil, p.wrapError("CreateItem", err)
	}
	item, err := p.client.GetArticle(ctx, strconv.Itoa(id))
	if err != nil {
		return &Item{ID: strconv.Itoa(id), Code: input.Code, Name: input.Description}, nil
	}
	return mapSimplBooksItem(item), nil
}

func (p *simplbooksProvider) ListItems(ctx context.Context, _ ListItemsInput) ([]Item, error) {
	items, err := p.client.ListAllArticles(ctx, nil, 1000)
	if err != nil {
		return nil, p.wrapError("ListItems", err)
	}
	result := make([]Item, len(items))
	for i := range items {
		result[i] = *mapSimplBooksItem(&items[i])
	}
	return result, nil
}

func (p *simplbooksProvider) UpdateItem(ctx context.Context, input UpdateItemInput) error {
	id := atoiOrZero(input.ID)
	if id == 0 {
		return &ProviderError{Provider: "simplbooks", Op: "UpdateItem", Err: ErrInvalidInput}
	}
	fields := map[string]any{}
	putIfPtr(fields, "name", input.Description)
	if input.SalesPrice != nil {
		fields["price_per_unit"] = input.SalesPrice.String()
	}
	return p.wrapError("UpdateItem", p.client.UpdateArticle(ctx, id, fields))
}

// --- Credit Notes ---

// CreateCreditNote creates a SimplBooks credit invoice by linking a new invoice
// to the original via credit_invoice_for. The rows carry the credited lines.
func (p *simplbooksProvider) CreateCreditNote(ctx context.Context, input CreateCreditNoteInput) (*Invoice, error) {
	invoice := map[string]any{
		"created":          formatSimplBooksDate(input.DocDate),
		"transaction_date": formatSimplBooksDate(input.DocDate),
		"due":              formatSimplBooksDate(input.DueDate),
		"row_sum_with_vat": false,
	}
	if id := atoiOrZero(input.CustomerID); id != 0 {
		invoice["client_id"] = id
	}
	if input.Currency != "" {
		invoice["currency_name"] = input.Currency
	}
	if input.RefNo != "" {
		invoice["reference"] = input.RefNo
	}
	if orig := atoiOrZero(input.OriginalInvoiceNo); orig != 0 {
		invoice["credit_invoice_for"] = orig
	}

	tasks := make([]map[string]any, 0, len(input.Lines))
	for _, line := range input.Lines {
		tasks = append(tasks, map[string]any{"Task": simplbooksRow(line)})
	}

	id, err := p.client.CreateInvoice(ctx, map[string]any{
		"Invoice": invoice,
		"Tasks":   tasks,
	})
	if err != nil {
		return nil, p.wrapError("CreateCreditNote", err)
	}
	return p.GetInvoice(ctx, strconv.Itoa(id))
}

// --- Purchases ---
//
// SimplBooks exposes purchases/* endpoints, but the purchase flow is not needed
// for the klubio sales-side integration and its response shape is not yet
// verified against a live tenant. Matching Excellent Books' surface, these are
// left unimplemented for now.

func (p *simplbooksProvider) CreatePurchase(_ context.Context, _ CreatePurchaseInput) (*PurchaseInvoice, error) {
	return nil, p.wrapError("CreatePurchase", fmt.Errorf("not yet implemented"))
}

func (p *simplbooksProvider) GetPurchase(_ context.Context, _ string) (*PurchaseInvoice, error) {
	return nil, p.wrapError("GetPurchase", fmt.Errorf("not yet implemented"))
}

func (p *simplbooksProvider) ListPurchases(_ context.Context, _ ListPurchasesInput) ([]PurchaseInvoice, error) {
	// No verified purchases/list mapping yet — return empty rather than error so
	// callers treat it as "no purchases surfaced".
	return nil, nil
}

func (p *simplbooksProvider) DeletePurchase(_ context.Context, _ string) error {
	return p.wrapError("DeletePurchase", fmt.Errorf("not yet implemented"))
}

// --- Reference data ---

func (p *simplbooksProvider) ListTaxes(ctx context.Context) ([]Tax, error) {
	types, err := p.client.ListVatTypes(ctx)
	if err != nil {
		return nil, p.wrapError("ListTaxes", err)
	}
	taxes := make([]Tax, 0, len(types))
	for _, t := range types {
		if !t.Active || !t.IsSales {
			continue
		}
		id := strconv.Itoa(t.ID)
		taxes = append(taxes, Tax{
			ID:   id,
			Code: id,
			Name: t.Name,
			Pct:  t.VatPercent.Decimal,
		})
	}
	return taxes, nil
}

func (p *simplbooksProvider) ListAccounts(ctx context.Context) ([]Account, error) {
	accs, err := p.client.ListAllFinancialAccounts(ctx, 200)
	if err != nil {
		return nil, p.wrapError("ListAccounts", err)
	}
	accounts := make([]Account, 0, len(accs))
	for _, a := range accs {
		if a.Code == "" {
			continue
		}
		accounts = append(accounts, Account{
			ID:     a.Code,
			Code:   a.Code,
			Name:   a.Name,
			Active: a.Active,
		})
	}
	return accounts, nil
}

// ListBanks returns SimplBooks' bank accounts and cash registers (the
// income_accounts register).
func (p *simplbooksProvider) ListBanks(ctx context.Context) ([]Bank, error) {
	accs, err := p.client.ListAllIncomeAccounts(ctx, 200)
	if err != nil {
		return nil, p.wrapError("ListBanks", err)
	}
	banks := make([]Bank, 0, len(accs))
	for _, a := range accs {
		if !a.Active {
			continue
		}
		banks = append(banks, Bank{
			ID:          strconv.Itoa(a.ID),
			Name:        a.AccountName,
			Description: a.AccountDescription,
			IBAN:        a.IbanNumber,
		})
	}
	return banks, nil
}

// ListPaymentTerms is unsupported — SimplBooks has no payment-term register API.
func (p *simplbooksProvider) ListPaymentTerms(_ context.Context) ([]PaymentTerm, error) {
	return nil, nil
}

// ListDimensions is unsupported — SimplBooks has no dimensions list API.
func (p *simplbooksProvider) ListDimensions(_ context.Context) (*DimensionList, error) {
	return &DimensionList{}, nil
}

// --- Reports ---

func (p *simplbooksProvider) CustomerDebts(_ context.Context, _ string, _ *int) ([]CustomerDebt, error) {
	return nil, p.wrapError("CustomerDebts", fmt.Errorf("not yet implemented"))
}

// --- Sync ---

func (p *simplbooksProvider) ListInvoicesSince(ctx context.Context, since, until time.Time) ([]Invoice, error) {
	return p.ListInvoices(ctx, ListInvoicesInput{PeriodStart: since, PeriodEnd: until})
}

func (p *simplbooksProvider) ListPaymentsSince(ctx context.Context, since, until time.Time) ([]Payment, error) {
	return p.ListPayments(ctx, ListPaymentsInput{PeriodStart: since, PeriodEnd: until})
}

// --- Mapping helpers ---

func simplbooksRow(line CreateInvoiceLineInput) map[string]any {
	row := map[string]any{
		"amount":         line.Quantity.String(),
		"price_per_unit": line.UnitPrice.String(),
	}
	name := line.Description
	if name == "" {
		name = line.Code
	}
	row["name"] = name
	if id := atoiOrZero(line.Code); id != 0 {
		row["article_id"] = id
	}
	if line.Description != "" {
		row["contents"] = line.Description
	}
	if id := atoiOrZero(line.TaxID); id != 0 {
		row["vat_type_id"] = id
	}
	if id := atoiOrZero(line.AccountCode); id != 0 {
		row["income_account_id"] = id
	}
	if line.UOMName != "" {
		row["unit"] = line.UOMName
	}
	return row
}

func mapSimplBooksInvoice(inv *simplbooks.Invoice) *Invoice {
	var lines []InvoiceLine
	for _, row := range inv.Rows {
		lines = append(lines, InvoiceLine{
			ID:            strconv.Itoa(row.ID),
			Description:   firstNonEmpty(row.Contents, row.Name),
			Quantity:      row.Amount.Decimal,
			UnitPrice:     row.PricePerUnit.Decimal,
			TaxID:         itoaOrEmpty(row.VatTypeID),
			AmountExclVat: row.Amount.Decimal.Mul(row.PricePerUnit.Decimal),
			AccountCode:   itoaOrEmpty(row.IncomeAccountID),
		})
	}

	paid := parseSimplBooksDate(inv.Paid)
	status := InvoiceStatusUnpaid
	isPaid := !paid.IsZero()
	if isPaid {
		status = InvoiceStatusPaid
	} else if !inv.Remnant.Decimal.IsZero() && inv.Remnant.Decimal.LessThan(inv.TotalSum.Decimal) {
		status = InvoiceStatusPartial
	}

	return &Invoice{
		ID:           strconv.Itoa(inv.ID),
		Number:       inv.Number,
		CustomerName: inv.ClientName,
		CustomerID:   itoaOrEmpty(inv.ClientID),
		DocDate:      parseSimplBooksDate(inv.Created),
		DueDate:      parseSimplBooksDate(inv.Due),
		TotalAmount:  inv.TotalSum.Decimal,
		TaxAmount:    inv.Vat.Decimal,
		Currency:     inv.CurrencyName,
		Paid:         isPaid,
		Status:       status,
		ReferenceNo:  inv.Reference,
		Lines:        lines,
	}
}

func mapSimplBooksCustomer(c *simplbooks.ClientRecord) *Customer {
	return &Customer{
		ID:          strconv.Itoa(c.ID),
		Name:        c.Name,
		RegNo:       c.RegNo,
		VATRegNo:    c.VatNo,
		Email:       c.Email,
		Phone:       c.Phone,
		Address:     c.AddressStreet,
		City:        c.AddressCity,
		County:      c.AddressCounty,
		PostalCode:  c.AddressPostalCode,
		CountryCode: c.AddressCountry,
		HomePage:    c.Webpage,
	}
}

func mapSimplBooksItem(a *simplbooks.Article) *Item {
	itemType := ItemTypeService
	if a.IsInventory {
		itemType = ItemTypeStock
	}
	return &Item{
		ID:            strconv.Itoa(a.ID),
		Code:          a.Code,
		Name:          a.Name,
		Description:   firstNonEmpty(a.Contents, a.Name),
		Type:          itemType,
		UnitOfMeasure: a.Unit,
		SalesPrice:    a.PricePerUnit.Decimal,
		TaxID:         itoaOrEmpty(a.SalesVatTypeID),
	}
}

// --- small utilities ---

func formatSimplBooksDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(simplbooksDateFormat)
}

// parseSimplBooksDate parses a "YYYY-MM-DD" date, returning the zero time for
// empty values and SimplBooks' "0000-00-00" unset sentinel.
func parseSimplBooksDate(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" || s == "0000-00-00" {
		return time.Time{}
	}
	t, err := time.Parse(simplbooksDateFormat, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

// inDateRange reports whether t falls within [start, end] (inclusive). Zero
// bounds are treated as open. A zero t (unparseable/unset) is never excluded.
func inDateRange(t, start, end time.Time) bool {
	if t.IsZero() {
		return true
	}
	if !start.IsZero() && t.Before(start) {
		return false
	}
	if !end.IsZero() && t.After(end) {
		return false
	}
	return true
}

func atoiOrZero(s string) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0
	}
	return n
}

func itoaOrEmpty(n int) string {
	if n == 0 {
		return ""
	}
	return strconv.Itoa(n)
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

func putIf(m map[string]any, key, val string) {
	if val != "" {
		m[key] = val
	}
}

func putIfPtr(m map[string]any, key string, val *string) {
	if val != nil {
		m[key] = *val
	}
}

// wrapError converts a SimplBooks APIError into the package's sentinel errors.
func (p *simplbooksProvider) wrapError(op string, err error) error {
	if err == nil {
		return nil
	}
	var apiErr *simplbooks.APIError
	if errors.As(err, &apiErr) {
		var sentinel error
		switch apiErr.StatusCode {
		case 401, 403:
			sentinel = ErrAuthFailed
		case 429:
			sentinel = ErrRateLimit
		case 400:
			// SimplBooks reports "does not exist" lookups as 400, not 404.
			if messagesContain(apiErr.Messages, "does not exist", "not found") {
				sentinel = ErrNotFound
			} else {
				sentinel = err
			}
		case 404:
			sentinel = ErrNotFound
		default:
			sentinel = err
		}
		return &ProviderError{Provider: "simplbooks", Op: op, Err: sentinel}
	}
	return &ProviderError{Provider: "simplbooks", Op: op, Err: err}
}

func messagesContain(msgs []string, needles ...string) bool {
	for _, m := range msgs {
		lm := strings.ToLower(m)
		for _, n := range needles {
			if strings.Contains(lm, n) {
				return true
			}
		}
	}
	return false
}
