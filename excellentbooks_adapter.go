package accounting

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/qbitsoftware/accounting-service/excellentbooks"
	"github.com/shopspring/decimal"
)

const excellentDateFormat = "2006-01-02"

// excellentProvider implements Provider using the Excellent Books API.
type excellentProvider struct {
	client *excellentbooks.Client
}

func newExcellentProvider(cfg Config) *excellentProvider {
	baseURL := strings.TrimRight(cfg.Extra["base_url"], "/")
	if baseURL != "" && !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
		baseURL = "https://" + baseURL
	}
	return &excellentProvider{
		client: excellentbooks.New(excellentbooks.Config{
			BaseURL:    baseURL,
			Username:   cfg.APIID,
			Password:   cfg.APIKey,
			HTTPClient: cfg.HTTPClient,
		}),
	}
}

func (p *excellentProvider) TestConnection(ctx context.Context) error {
	_, _, err := p.client.ListCustomers(ctx, excellentbooks.ListParams{Limit: 1})
	return p.wrapError("TestConnection", err)
}

// --- Invoices ---

func (p *excellentProvider) CreateInvoice(ctx context.Context, input CreateInvoiceInput) (*Invoice, error) {
	fields := map[string]string{
		"set_field.InvDate":  formatExcellentDate(input.DocDate),
		"set_field.CustCode": input.CustomerID,
		"set_field.PayDeal":  deriveDaysUntilDue(input.DocDate, input.DueDate),
	}
	if input.Currency != "" {
		fields["set_field.CurncyCode"] = input.Currency
	}
	if input.Comment != "" {
		fields["set_field.InvComment"] = input.Comment
	}
	if input.RefNo != "" {
		fields["set_field.RefStr"] = input.RefNo
	}

	for i, line := range input.Lines {
		prefix := fmt.Sprintf("set_row_field.%d", i)
		fields[prefix+".ArtCode"] = line.Code
		fields[prefix+".Quant"] = line.Quantity.String()
		fields[prefix+".Price"] = line.UnitPrice.String()
		if line.TaxID != "" {
			fields[prefix+".VATCode"] = line.TaxID
		}
		if line.AccountCode != "" {
			fields[prefix+".SalesAcc"] = line.AccountCode
		}
		if line.Description != "" {
			fields[prefix+".Spec"] = line.Description
		}
	}

	if input.AutoConfirm {
		fields["set_field.OKFlag"] = "1"
	}

	inv, err := p.client.CreateInvoice(ctx, fields)
	if err != nil {
		return nil, p.wrapError("CreateInvoice", err)
	}

	return mapExcellentInvoice(inv), nil
}

func (p *excellentProvider) GetInvoice(ctx context.Context, id string) (*Invoice, error) {
	inv, err := p.client.GetInvoice(ctx, id)
	if err != nil {
		return nil, p.wrapError("GetInvoice", err)
	}
	return mapExcellentInvoice(inv), nil
}

func (p *excellentProvider) GetInvoicePDF(_ context.Context, _ string, _ bool) (*InvoicePDF, error) {
	return nil, p.wrapError("GetInvoicePDF", fmt.Errorf("not supported by Excellent Books API"))
}

func (p *excellentProvider) ListInvoices(ctx context.Context, input ListInvoicesInput) ([]Invoice, error) {
	params := excellentbooks.ListParams{Limit: 5000}
	if !input.PeriodStart.IsZero() {
		params.Sort = "InvDate"
		params.Range = formatExcellentDate(input.PeriodStart) + ":"
		if !input.PeriodEnd.IsZero() {
			params.Range += formatExcellentDate(input.PeriodEnd)
		}
	}

	items, _, err := p.client.ListInvoices(ctx, params)
	if err != nil {
		return nil, p.wrapError("ListInvoices", err)
	}

	invoices := make([]Invoice, len(items))
	for i := range items {
		invoices[i] = *mapExcellentInvoice(&items[i])
	}
	return invoices, nil
}

func (p *excellentProvider) DeleteInvoice(_ context.Context, _ string) error {
	return p.wrapError("DeleteInvoice", fmt.Errorf("not supported by Excellent Books API"))
}

func (p *excellentProvider) FindInvoiceByRef(ctx context.Context, refStr string) (*Invoice, error) {
	items, _, err := p.client.ListInvoices(ctx, excellentbooks.ListParams{
		Limit:  1,
		Filter: map[string]string{"RefStr": refStr},
	})
	if err != nil {
		return nil, p.wrapError("FindInvoiceByRef", err)
	}
	if len(items) == 0 {
		return nil, &ProviderError{Provider: "excellentbooks", Op: "FindInvoiceByRef", Err: ErrNotFound}
	}
	return mapExcellentInvoice(&items[0]), nil
}

// --- Customers ---

func (p *excellentProvider) CreateCustomer(ctx context.Context, input CreateCustomerInput) (*Customer, error) {
	fields := map[string]string{
		"set_field.Name": input.Name,
	}
	if input.Email != "" {
		fields["set_field.eMail"] = input.Email
	}
	if input.Phone != "" {
		fields["set_field.Phone"] = input.Phone
	}
	if input.RegNo != "" {
		fields["set_field.RegNr1"] = input.RegNo
	}
	if input.VATRegNo != "" {
		fields["set_field.VATNr"] = input.VATRegNo
	}
	if input.CountryCode != "" {
		fields["set_field.CountryCode"] = input.CountryCode
	}
	if input.Currency != "" {
		fields["set_field.CurncyCode"] = input.Currency
	}
	if input.Address != "" {
		fields["set_field.InvAddr0"] = input.Address
	}
	if input.City != "" {
		fields["set_field.InvAddr1"] = input.City
	}
	if input.PostalCode != "" {
		fields["set_field.InvAddr2"] = input.PostalCode
	}
	if input.PaymentDays != nil {
		fields["set_field.PayDeal"] = strconv.Itoa(*input.PaymentDays)
	}
	if input.Contact != "" {
		fields["set_field.Person"] = input.Contact
	}

	cust, err := p.client.CreateCustomer(ctx, fields)
	if err != nil {
		return nil, p.wrapError("CreateCustomer", err)
	}

	return mapExcellentCustomer(cust), nil
}

func (p *excellentProvider) UpdateCustomer(ctx context.Context, input UpdateCustomerInput) error {
	fields := map[string]string{}
	if input.Name != nil {
		fields["set_field.Name"] = *input.Name
	}
	if input.Email != nil {
		fields["set_field.eMail"] = *input.Email
	}
	if input.Phone != nil {
		fields["set_field.Phone"] = *input.Phone
	}
	if input.Address != nil {
		fields["set_field.InvAddr0"] = *input.Address
	}
	if input.City != nil {
		fields["set_field.InvAddr1"] = *input.City
	}
	if input.PostalCode != nil {
		fields["set_field.InvAddr2"] = *input.PostalCode
	}
	if input.CountryCode != nil {
		fields["set_field.CountryCode"] = *input.CountryCode
	}
	if input.RegNo != nil {
		fields["set_field.RegNr1"] = *input.RegNo
	}
	if input.VATRegNo != nil {
		fields["set_field.VATNr"] = *input.VATRegNo
	}

	return p.wrapError("UpdateCustomer", p.client.UpdateCustomer(ctx, input.ID, fields))
}

func (p *excellentProvider) ListCustomers(ctx context.Context, _ ListCustomersInput) ([]Customer, error) {
	items, _, err := p.client.ListCustomers(ctx, excellentbooks.ListParams{Limit: 5000})
	if err != nil {
		return nil, p.wrapError("ListCustomers", err)
	}

	customers := make([]Customer, len(items))
	for i := range items {
		customers[i] = *mapExcellentCustomer(&items[i])
	}
	return customers, nil
}

func (p *excellentProvider) FindCustomerByEmail(ctx context.Context, email string) (*Customer, error) {
	items, _, err := p.client.ListCustomers(ctx, excellentbooks.ListParams{Limit: 5000})
	if err != nil {
		return nil, p.wrapError("FindCustomerByEmail", err)
	}

	email = strings.ToLower(strings.TrimSpace(email))
	for _, item := range items {
		if strings.ToLower(strings.TrimSpace(item.Email)) == email {
			return mapExcellentCustomer(&item), nil
		}
	}

	return nil, &ProviderError{Provider: "excellentbooks", Op: "FindCustomerByEmail", Err: ErrNotFound}
}

// --- Payments ---

func (p *excellentProvider) CreatePayment(ctx context.Context, input CreatePaymentInput) error {
	fields := map[string]string{
		"set_field.TransDate": formatExcellentDate(input.PaymentDate),
		"set_field.OKFlag":    "1",
	}
	if input.BankID != "" {
		fields["set_field.PayMode"] = input.BankID
	}
	if input.Currency != "" {
		fields["set_field.PayCurCode"] = input.Currency
	}

	fields["set_row_field.0.stp"] = "1"
	fields["set_row_field.0.InvoiceNr"] = input.InvoiceNo
	fields["set_row_field.0.CustCode"] = input.CustomerName
	if !input.Amount.IsZero() {
		fields["set_row_field.0.RecVal"] = input.Amount.String()
	}
	fields["set_row_field.0.PayDate"] = formatExcellentDate(input.PaymentDate)

	_, err := p.client.CreateReceipt(ctx, fields)
	return p.wrapError("CreatePayment", err)
}

func (p *excellentProvider) ListPayments(ctx context.Context, input ListPaymentsInput) ([]Payment, error) {
	params := excellentbooks.ListParams{Limit: 5000}
	if !input.PeriodStart.IsZero() {
		params.Sort = "TransDate"
		params.Range = formatExcellentDate(input.PeriodStart) + ":"
		if !input.PeriodEnd.IsZero() {
			params.Range += formatExcellentDate(input.PeriodEnd)
		}
	}

	receipts, _, err := p.client.ListReceipts(ctx, params)
	if err != nil {
		return nil, p.wrapError("ListPayments", err)
	}

	payments := make([]Payment, len(receipts))
	for i, r := range receipts {
		amount := decimal.Zero
		var links []PaymentInvoiceLink
		for _, row := range r.Rows {
			rowAmt, _ := decimal.NewFromString(row.RecVal)
			amount = amount.Add(rowAmt)
			if row.InvoiceNr != "" {
				links = append(links, PaymentInvoiceLink{
					InvoiceID: row.InvoiceNr,
					InvoiceNo: row.InvoiceNr,
					Amount:    rowAmt,
				})
			}
		}
		payments[i] = Payment{
			ID:            r.SerNr,
			DocumentNo:    r.SerNr,
			DocumentDate:  parseExcellentDate(r.TransDate),
			Amount:        amount,
			Currency:      r.PayCurCode,
			Direction:     PaymentDirectionCustomer,
			InvoiceLinks:  links,
		}
	}
	return payments, nil
}

func (p *excellentProvider) DeletePayment(_ context.Context, _ string) error {
	return p.wrapError("DeletePayment", fmt.Errorf("not supported by Excellent Books API"))
}

// --- Items ---

func (p *excellentProvider) CreateItem(ctx context.Context, input CreateItemInput) (*Item, error) {
	fields := map[string]string{
		"set_field.Code": input.Code,
		"set_field.Name": input.Description,
	}
	if input.UnitOfMeasure != "" {
		fields["set_field.Unittext"] = input.UnitOfMeasure
	}
	if !input.SalesPrice.IsZero() {
		fields["set_field.UPrice1"] = input.SalesPrice.String()
	}
	if input.TaxID != "" {
		fields["set_field.VATCode"] = input.TaxID
	}
	if input.SalesAccountCode != "" {
		fields["set_field.SalesAcc"] = input.SalesAccountCode
	}
	// ItemType 1 = stock. Service/plain items use the default (no value needed).
	// ItemType 2 = Recipe/Kit in StandardBooks — do NOT send for services.
	if input.Type == ItemTypeStock {
		fields["set_field.ItemType"] = "1"
	}

	item, err := p.client.CreateItem(ctx, fields)
	if err != nil {
		return nil, p.wrapError("CreateItem", err)
	}

	return mapExcellentItem(item), nil
}

func (p *excellentProvider) ListItems(ctx context.Context, _ ListItemsInput) ([]Item, error) {
	items, _, err := p.client.ListItems(ctx, excellentbooks.ListParams{Limit: 5000})
	if err != nil {
		return nil, p.wrapError("ListItems", err)
	}

	result := make([]Item, len(items))
	for i := range items {
		result[i] = *mapExcellentItem(&items[i])
	}
	return result, nil
}

func (p *excellentProvider) UpdateItem(ctx context.Context, input UpdateItemInput) error {
	fields := map[string]string{}
	if input.Description != nil {
		fields["set_field.Name"] = *input.Description
	}
	if input.SalesPrice != nil {
		fields["set_field.UPrice1"] = input.SalesPrice.String()
	}
	if input.TaxID != nil {
		fields["set_field.VATCode"] = *input.TaxID
	}

	_, err := p.client.CreateItem(ctx, fields) // PATCH via code
	return p.wrapError("UpdateItem", err)
}

// --- Credit Notes ---

func (p *excellentProvider) CreateCreditNote(ctx context.Context, input CreateCreditNoteInput) (*Invoice, error) {
	fields := map[string]string{
		"set_field.InvDate":  formatExcellentDate(input.DocDate),
		"set_field.CustCode": input.CustomerID,
	}
	if input.Currency != "" {
		fields["set_field.CurncyCode"] = input.Currency
	}
	if input.OriginalInvoiceNo != "" {
		fields["set_field.CredInv"] = input.OriginalInvoiceNo
	}

	for i, line := range input.Lines {
		prefix := fmt.Sprintf("set_row_field.%d", i)
		fields[prefix+".stp"] = "3" // credit row type
		fields[prefix+".ArtCode"] = line.Code
		fields[prefix+".Quant"] = line.Quantity.String()
		fields[prefix+".Price"] = line.UnitPrice.String()
		if line.TaxID != "" {
			fields[prefix+".VATCode"] = line.TaxID
		}
	}
	fields["set_field.OKFlag"] = "1"

	inv, err := p.client.CreateInvoice(ctx, fields)
	if err != nil {
		return nil, p.wrapError("CreateCreditNote", err)
	}
	return mapExcellentInvoice(inv), nil
}

// --- Purchases ---

func (p *excellentProvider) CreatePurchase(_ context.Context, _ CreatePurchaseInput) (*PurchaseInvoice, error) {
	return nil, p.wrapError("CreatePurchase", fmt.Errorf("not yet implemented"))
}

func (p *excellentProvider) GetPurchase(_ context.Context, _ string) (*PurchaseInvoice, error) {
	return nil, p.wrapError("GetPurchase", fmt.Errorf("not yet implemented"))
}

func (p *excellentProvider) ListPurchases(ctx context.Context, input ListPurchasesInput) ([]PurchaseInvoice, error) {
	params := excellentbooks.ListParams{Limit: 5000}
	if !input.PeriodStart.IsZero() {
		params.Sort = "InvDate"
		params.Range = formatExcellentDate(input.PeriodStart) + ":"
		if !input.PeriodEnd.IsZero() {
			params.Range += formatExcellentDate(input.PeriodEnd)
		}
	}

	items, _, err := p.client.ListPurchases(ctx, params)
	if err != nil {
		return nil, p.wrapError("ListPurchases", err)
	}

	purchases := make([]PurchaseInvoice, len(items))
	for i, item := range items {
		total, _ := decimal.NewFromString(item.PayVal)
		tax, _ := decimal.NewFromString(item.VATVal)
		purchases[i] = PurchaseInvoice{
			ID:          item.SerNr,
			Number:      item.InvoiceNr,
			VendorName:  item.VEName,
			VendorID:    item.VECode,
			DocDate:     parseExcellentDate(item.InvDate),
			DueDate:     parseExcellentDate(item.DueDate),
			TotalAmount: total,
			TaxAmount:   tax,
			Currency:    item.CurncyCode,
			Paid:        false,
			Status:      InvoiceStatusUnpaid,
			ReferenceNo: item.RefStr,
		}
	}
	return purchases, nil
}

func (p *excellentProvider) DeletePurchase(_ context.Context, _ string) error {
	return p.wrapError("DeletePurchase", fmt.Errorf("not supported by Excellent Books API"))
}

// --- Reference data ---

func (p *excellentProvider) ListTaxes(_ context.Context) ([]Tax, error) {
	return nil, p.wrapError("ListTaxes", fmt.Errorf("not yet implemented"))
}

func (p *excellentProvider) ListAccounts(_ context.Context) ([]Account, error) {
	return nil, p.wrapError("ListAccounts", fmt.Errorf("not yet implemented"))
}

func (p *excellentProvider) ListDimensions(_ context.Context) (*DimensionList, error) {
	return nil, p.wrapError("ListDimensions", fmt.Errorf("not yet implemented"))
}

// --- Reports ---

func (p *excellentProvider) CustomerDebts(_ context.Context, _ string, _ *int) ([]CustomerDebt, error) {
	return nil, p.wrapError("CustomerDebts", fmt.Errorf("not yet implemented"))
}

// --- Sync ---

func (p *excellentProvider) ListInvoicesSince(ctx context.Context, since time.Time, until time.Time) ([]Invoice, error) {
	return p.ListInvoices(ctx, ListInvoicesInput{PeriodStart: since, PeriodEnd: until})
}

func (p *excellentProvider) ListPaymentsSince(_ context.Context, _ time.Time, _ time.Time) ([]Payment, error) {
	return nil, p.wrapError("ListPaymentsSince", fmt.Errorf("not yet implemented"))
}

// --- Mapping helpers ---

func mapExcellentInvoice(inv *excellentbooks.Invoice) *Invoice {
	total, _ := decimal.NewFromString(inv.Sum4)
	tax, _ := decimal.NewFromString(inv.Sum3)
	net, _ := decimal.NewFromString(inv.Sum1)

	var lines []InvoiceLine
	for _, row := range inv.Rows {
		qty, _ := decimal.NewFromString(row.Quant)
		price, _ := decimal.NewFromString(row.Price)
		sum, _ := decimal.NewFromString(row.Sum)
		lines = append(lines, InvoiceLine{
			ID:            row.RowNumber,
			Description:   row.Spec,
			Quantity:      qty,
			UnitPrice:     price,
			TaxID:         row.VATCode,
			AmountExclVat: sum,
			AccountCode:   row.SalesAcc,
		})
	}

	ref := inv.RefStr
	if ref == "" {
		ref = inv.CalcFinRef
	}

	_ = net // available if needed

	return &Invoice{
		ID:           inv.SerNr,
		Number:       inv.SerNr,
		CustomerName: inv.Addr0,
		CustomerID:   inv.CustCode,
		DocDate:      parseExcellentDate(inv.InvDate),
		DueDate:      parseExcellentDate(inv.PayDate),
		TotalAmount:  total,
		TaxAmount:    tax,
		Currency:     inv.CurncyCode,
		Paid:         false, // Excellent Books doesn't expose paid status in invoice list
		Status:       InvoiceStatusUnpaid,
		ReferenceNo:  ref,
		Lines:        lines,
	}
}

func mapExcellentCustomer(cust *excellentbooks.Customer) *Customer {
	payDays, _ := strconv.Atoi(cust.PayDeal)

	return &Customer{
		ID:          cust.Code,
		Name:        cust.Name,
		RegNo:       cust.RegNr1,
		VATRegNo:    cust.VATNr,
		Email:       cust.Email,
		Phone:       cust.Phone,
		Address:     cust.InvAddr0,
		City:        cust.InvAddr1,
		PostalCode:  cust.InvAddr2,
		CountryCode: cust.CountryCode,
		Currency:    cust.CurncyCode,
		PaymentDays: payDays,
		Contact:     cust.Person,
		HomePage:    cust.WWWAddr,
	}
}

func mapExcellentItem(item *excellentbooks.Item) *Item {
	price, _ := decimal.NewFromString(item.UPrice1)

	itemType := ItemTypeService
	if item.ItemType == "1" {
		itemType = ItemTypeStock
	}

	return &Item{
		ID:            item.Code,
		Code:          item.Code,
		Name:          item.Name,
		Description:   item.Name,
		Type:          itemType,
		UnitOfMeasure: item.Unittext,
		SalesPrice:    price,
		TaxID:         item.VATCode,
	}
}

func formatExcellentDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(excellentDateFormat)
}

func parseExcellentDate(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(excellentDateFormat, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

// deriveDaysUntilDue returns the number of days between doc date and due date as a string.
func deriveDaysUntilDue(docDate, dueDate time.Time) string {
	if dueDate.IsZero() || docDate.IsZero() {
		return "14" // default
	}
	days := int(dueDate.Sub(docDate).Hours() / 24)
	if days <= 0 {
		return "0"
	}
	return strconv.Itoa(days)
}

// wrapError converts Excellent Books APIError into sentinel errors.
func (p *excellentProvider) wrapError(op string, err error) error {
	if err == nil {
		return nil
	}

	var apiErr *excellentbooks.APIError
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
		return &ProviderError{Provider: "excellentbooks", Op: op, Err: sentinel}
	}

	return &ProviderError{Provider: "excellentbooks", Op: op, Err: err}
}
