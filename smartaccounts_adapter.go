package accounting

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/qbitsoftware/accounting-service/smartaccounts"
	"github.com/shopspring/decimal"
)

// smartProvider implements Provider using the SmartAccounts API.
type smartProvider struct {
	client *smartaccounts.Client
}

func newSmartAccountsProvider(cfg Config) *smartProvider {
	return &smartProvider{
		client: smartaccounts.New(smartaccounts.Config{
			Host:        cfg.Region, // optional host override; empty → default host
			APIKey:      cfg.APIID,  // public API key
			SecretKey:   cfg.APIKey, // private/secret key (HMAC key)
			Language:    cfg.Extra["language"],
			NettingBank: cfg.Extra["netting_bank"], // empty disables auto-settle on credit notes
			HTTPClient:  cfg.HTTPClient,
		}),
	}
}

func (p *smartProvider) TestConnection(ctx context.Context) error {
	_, err := p.client.ListVatPcs(ctx)
	return p.wrapError("TestConnection", err)
}

// --- SmartAccounts-specific date helpers (dd.MM.yyyy) ---

const saDateFormat = "02.01.2006"

func saFormatDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(saDateFormat)
}

func saParseDate(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, _ := time.Parse(saDateFormat, s)
	return t
}

// --- Invoices ---

func buildSARows(lines []CreateInvoiceLineInput) []smartaccounts.InvoiceRowInput {
	rows := make([]smartaccounts.InvoiceRowInput, len(lines))
	for i, l := range lines {
		rows[i] = smartaccounts.InvoiceRowInput{
			Code:         l.Code,
			Description:  l.Description,
			Price:        l.UnitPrice,
			Quantity:     l.Quantity,
			Unit:         l.UOMName,
			VatPc:        l.TaxID, // SmartAccounts keys VAT by vatPc code
			AccountSales: l.AccountCode,
			ObjectID:     l.ProjectCode,
		}
	}
	return rows
}

func totalPtr(d decimal.Decimal) *decimal.Decimal {
	if d.IsZero() {
		return nil
	}
	return &d
}

func (p *smartProvider) CreateInvoice(ctx context.Context, input CreateInvoiceInput) (*Invoice, error) {
	clientID, err := p.resolveClientID(ctx, input.CustomerID, input.CustomerName, input.CustomerRegNo)
	if err != nil {
		return nil, p.wrapError("CreateInvoice", err)
	}

	req := smartaccounts.CreateInvoiceRequest{
		ClientID:        clientID,
		Date:            saFormatDate(input.DocDate),
		DueDate:         saFormatDate(input.DueDate),
		InvoiceNumber:   input.InvoiceNo,
		ReferenceNumber: input.RefNo,
		Currency:        input.Currency,
		TotalAmount:     totalPtr(input.TotalAmount),
		Comment:         input.Comment,
		Rows:            buildSARows(input.Lines),
	}

	resp, err := p.client.CreateInvoice(ctx, req)
	if err != nil {
		return nil, p.wrapError("CreateInvoice", err)
	}

	return &Invoice{
		ID:           resp.InvoiceID,
		Number:       string(resp.InvoiceNumber),
		CustomerName: input.CustomerName,
		CustomerID:   resp.ClientID,
		DocDate:      input.DocDate,
		DueDate:      input.DueDate,
		TotalAmount:  resp.TotalAmount,
		TaxAmount:    resp.VatAmount,
		Currency:     input.Currency,
		ReferenceNo:  string(resp.ReferenceNumber),
		Status:       InvoiceStatusUnpaid,
	}, nil
}

func (p *smartProvider) GetInvoice(ctx context.Context, id string) (*Invoice, error) {
	item, err := p.client.GetInvoice(ctx, id)
	if err != nil {
		return nil, p.wrapError("GetInvoice", err)
	}
	inv := mapSAInvoice(*item)
	return &inv, nil
}

func (p *smartProvider) GetInvoicePDF(ctx context.Context, id string, _ bool) (*InvoicePDF, error) {
	resp, err := p.client.GetInvoicePDF(ctx, id)
	if err != nil {
		return nil, p.wrapError("GetInvoicePDF", err)
	}
	content, err := base64.StdEncoding.DecodeString(resp.FileContent)
	if err != nil {
		return nil, p.wrapError("GetInvoicePDF", fmt.Errorf("decode pdf: %w", err))
	}
	return &InvoicePDF{FileName: resp.FileName, FileContent: content}, nil
}

func (p *smartProvider) ListInvoices(ctx context.Context, input ListInvoicesInput) ([]Invoice, error) {
	items, _, err := p.client.ListInvoices(ctx, smartaccounts.ListInvoicesParams{
		DateFrom:  saFormatDate(input.PeriodStart),
		DateTo:    saFormatDate(input.PeriodEnd),
		FetchRows: true,
	})
	if err != nil {
		return nil, p.wrapError("ListInvoices", err)
	}
	return mapSAInvoices(items), nil
}

func (p *smartProvider) FindInvoiceByRef(ctx context.Context, refStr string) (*Invoice, error) {
	item, err := p.client.FindInvoiceByNumber(ctx, refStr)
	if err != nil {
		return nil, p.wrapError("FindInvoiceByRef", err)
	}
	inv := mapSAInvoice(*item)
	return &inv, nil
}

func (p *smartProvider) DeleteInvoice(ctx context.Context, id string) error {
	return p.wrapError("DeleteInvoice", p.client.DeleteInvoice(ctx, id))
}

// --- Credit Notes ---

func (p *smartProvider) CreateCreditNote(ctx context.Context, input CreateCreditNoteInput) (*Invoice, error) {
	clientID, err := p.resolveClientID(ctx, input.CustomerID, input.CustomerName, input.CustomerRegNo)
	if err != nil {
		return nil, p.wrapError("CreateCreditNote", err)
	}

	// The caller hands us credit-note-convention lines (Merit shape): quantity
	// already negated and price negated to positive. SmartAccounts marks the
	// credit via Type=CRE and expects ordinary positive rows, so we negate the
	// quantity back to positive (the price is already positive).
	rows := buildSARows(input.Lines)
	for i := range rows {
		rows[i].Quantity = rows[i].Quantity.Neg()
	}

	req := smartaccounts.CreateInvoiceRequest{
		ClientID:        clientID,
		Type:            smartaccounts.InvoiceTypeCredit,
		Date:            saFormatDate(input.DocDate),
		DueDate:         saFormatDate(input.DueDate),
		InvoiceNumber:   input.InvoiceNo,
		ReferenceNumber: input.RefNo,
		Currency:        input.Currency,
		// TotalAmount is intentionally omitted: input.TotalAmount is a NET
		// subtotal (before tax), but SmartAccounts' totalAmount field is gross
		// (incl VAT) and is used for rounding reconciliation. Sending the net
		// value would book the entire VAT delta as a bogus roundAmount, so we
		// let SmartAccounts compute the total from the rows + vatPc instead.
		Comment: input.Comment,
		Rows:    rows,
	}

	// Link the credit invoice to the original so it offsets that invoice's
	// balance. The caller passes the original's number; resolve it to the SA id.
	// Fail (rather than create an unlinked, never-offsetting credit note) if the
	// original can't be uniquely resolved — the sync retries once the original
	// is present, mirroring how Excellent Books requires the parent first.
	if input.OriginalInvoiceNo != "" {
		orig, ferr := p.client.FindInvoiceByNumber(ctx, input.OriginalInvoiceNo)
		if ferr != nil {
			return nil, p.wrapError("CreateCreditNote", fmt.Errorf("cannot link credit note to original invoice %s: %w", input.OriginalInvoiceNo, ferr))
		}
		req.BaseForCreditInvoiceID = orig.ID

		// SA enforces that a credit invoice's date AND entry date must be on
		// or after the original's (CREDIT-INVOICE-EARLIER-THAN-BASE /
		// CREDIT-INVOICE-ENTRY-EARLIER-THAN-BASE). When the original is
		// future-dated (e.g. a plan-generated invoice for next month's billing
		// period that is credited "today"), bump the credit's Date and EntryDate
		// to the original's so SA accepts it. The local credit note keeps its
		// own DocDate; only the date sent to SA shifts.
		//
		// Also bump DueDate to match — otherwise SA computes the credit as
		// overdue from day one (due < date) and flags it red in its UI, even
		// though "due date" is meaningless for a credit invoice (no money is
		// owed by the customer, the credit offsets a balance).
		if origDate := saParseDate(string(orig.Date)); !origDate.IsZero() && origDate.After(input.DocDate) {
			bumped := saFormatDate(origDate)
			req.Date = bumped
			req.EntryDate = bumped
			if dueDate := saParseDate(req.DueDate); dueDate.IsZero() || dueDate.Before(origDate) {
				req.DueDate = bumped
			}
		}
	}

	resp, err := p.client.CreateInvoice(ctx, req)
	if err != nil {
		return nil, p.wrapError("CreateCreditNote", err)
	}

	// Auto-settle the credit against the original via the configured netting
	// bank, so both balances close immediately instead of lingering as open
	// items in SA. Best-effort: a settle failure does NOT fail the credit-note
	// creation (the document was created successfully and remains valid) — we
	// log loudly so the admin can settle manually if needed. Skipped silently
	// when no netting bank is configured.
	if req.BaseForCreditInvoiceID != "" && p.client.NettingBank() != "" {
		p.autoSettleCreditNote(ctx, resp, req.BaseForCreditInvoiceID, input)
	}

	return &Invoice{
		ID:           resp.InvoiceID,
		Number:       string(resp.InvoiceNumber),
		CustomerName: input.CustomerName,
		CustomerID:   resp.ClientID,
		DocDate:      input.DocDate,
		DueDate:      input.DueDate,
		Currency:     input.Currency,
		ReferenceNo:  string(resp.ReferenceNumber),
		Status:       InvoiceStatusUnpaid,
	}, nil
}

// autoSettleCreditNote nets a freshly-created credit against its original via
// a payment in the configured netting bank. Settles min(original.outstanding,
// credit.totalAmount) so we never over-settle either side; skipped if the
// original is already fully paid (outstanding == 0), in which case the credit
// represents a real refund liability that must be handled outside this flow.
func (p *smartProvider) autoSettleCreditNote(ctx context.Context, resp *smartaccounts.InvoiceResponse, originalID string, input CreateCreditNoteInput) {
	orig, err := p.client.GetInvoice(ctx, originalID)
	if err != nil {
		slog.Warn("smartaccounts: credit note created but auto-settle skipped — could not refetch original",
			"credit_id", resp.InvoiceID, "original_id", originalID, "error", err)
		return
	}
	if !orig.OutstandingAmount.IsPositive() {
		slog.Warn("smartaccounts: credit note created but auto-settle skipped — original is already fully paid; credit is a refund liability",
			"credit_id", resp.InvoiceID, "original_id", originalID, "original_outstanding", orig.OutstandingAmount.String())
		return
	}
	creditTotal := resp.TotalAmount.Abs()
	if !creditTotal.IsPositive() {
		return
	}
	settle := creditTotal
	if orig.OutstandingAmount.LessThan(settle) {
		settle = orig.OutstandingAmount
	}
	pay, err := p.client.SettleInvoiceAgainstCredit(ctx, resp.ClientID, originalID, resp.InvoiceID, settle, input.Currency, saFormatDate(input.DocDate))
	if err != nil {
		slog.Error("smartaccounts: credit note created but auto-settle FAILED — please settle manually in SA UI",
			"credit_id", resp.InvoiceID, "original_id", originalID, "settle_amount", settle.String(), "error", err)
		return
	}
	slog.Info("smartaccounts: credit note auto-settled",
		"credit_id", resp.InvoiceID, "original_id", originalID, "settle_amount", settle.String(), "payment_id", pay.PaymentID)
}

// --- Customers ---

func (p *smartProvider) CreateCustomer(ctx context.Context, input CreateCustomerInput) (*Customer, error) {
	req := smartaccounts.CreateClientRequest{
		Name:           input.Name,
		RegCode:        input.RegNo,
		VatNumber:      input.VATRegNo,
		InvoiceDueDate: input.PaymentDays,
		Address:        buildSAAddress(input.Address, input.City, input.County, input.PostalCode, input.CountryCode),
		Contacts:       buildSAContacts(input.Email, input.Phone),
	}

	resp, err := p.client.CreateClient(ctx, req)
	if err != nil {
		return nil, p.wrapError("CreateCustomer", err)
	}

	return &Customer{
		ID:          resp.ClientID,
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
		Contact:     input.Contact,
		RefNoBase:   resp.ReferenceNumber,
	}, nil
}

// UpdateCustomer fetches the existing client, applies the requested changes,
// and sends the full object — SmartAccounts edit requests must contain all
// fields, not just the changed ones.
func (p *smartProvider) UpdateCustomer(ctx context.Context, input UpdateCustomerInput) error {
	existing, err := p.client.GetClient(ctx, input.ID)
	if err != nil {
		return p.wrapError("UpdateCustomer", err)
	}

	addr := smartaccounts.Address{}
	if existing.Address != nil {
		addr = *existing.Address
	}
	email := contactValue(existing.Contacts, smartaccounts.ContactEmail)
	phone := contactValue(existing.Contacts, smartaccounts.ContactPhone)

	req := smartaccounts.CreateClientRequest{
		ID:        existing.ID,
		Name:      existing.Name,
		RegCode:   existing.RegCode,
		VatNumber: existing.VatNumber,
	}
	if input.Name != nil {
		req.Name = *input.Name
	}
	if input.RegNo != nil {
		req.RegCode = *input.RegNo
	}
	if input.VATRegNo != nil {
		req.VatNumber = *input.VATRegNo
	}
	if input.Address != nil {
		addr.Address1 = *input.Address
	}
	if input.City != nil {
		addr.City = *input.City
	}
	if input.PostalCode != nil {
		addr.PostalCode = *input.PostalCode
	}
	if input.CountryCode != nil {
		addr.Country = *input.CountryCode
	}
	if input.Email != nil {
		email = *input.Email
	}
	if input.Phone != nil {
		phone = *input.Phone
	}
	req.Address = &addr
	req.Contacts = buildSAContacts(email, phone)

	return p.wrapError("UpdateCustomer", p.client.EditClient(ctx, req))
}

func (p *smartProvider) ListCustomers(ctx context.Context, _ ListCustomersInput) ([]Customer, error) {
	items, err := p.client.ListClients(ctx, smartaccounts.ListClientsParams{})
	if err != nil {
		return nil, p.wrapError("ListCustomers", err)
	}
	customers := make([]Customer, len(items))
	for i, item := range items {
		customers[i] = mapSAClient(item)
	}
	return customers, nil
}

func (p *smartProvider) GetCustomer(ctx context.Context, id string) (*Customer, error) {
	item, err := p.client.GetClient(ctx, id)
	if err != nil {
		return nil, p.wrapError("GetCustomer", err)
	}
	c := mapSAClient(*item)
	return &c, nil
}

func (p *smartProvider) FindCustomerByEmail(ctx context.Context, email string) (*Customer, error) {
	items, err := p.client.ListClients(ctx, smartaccounts.ListClientsParams{})
	if err != nil {
		return nil, p.wrapError("FindCustomerByEmail", err)
	}
	email = strings.ToLower(strings.TrimSpace(email))
	for _, item := range items {
		if strings.ToLower(strings.TrimSpace(contactValue(item.Contacts, smartaccounts.ContactEmail))) == email {
			c := mapSAClient(item)
			return &c, nil
		}
	}
	return nil, &ProviderError{Provider: "smartaccounts", Op: "FindCustomerByEmail", Err: ErrNotFound}
}

// --- Payments ---

// CreatePayment records a payment against a client invoice. SmartAccounts links
// payments to invoices by invoice ID, so we resolve the invoice number to its
// ID first and reference the bank account by name (the BankID field carries the
// SmartAccounts bank-account name).
func (p *smartProvider) CreatePayment(ctx context.Context, input CreatePaymentInput) error {
	invoice, err := p.client.FindInvoiceByNumber(ctx, input.InvoiceNo)
	if err != nil {
		return p.wrapError("CreatePayment", err)
	}

	// Guard against silently booking a payment in a different currency than the
	// invoice it settles — SmartAccounts would not reconcile it and there is no
	// FX handling here. Empty currencies are treated as matching (defaults).
	payCurrency := strings.TrimSpace(input.Currency)
	invCurrency := strings.TrimSpace(invoice.Currency)
	if payCurrency != "" && invCurrency != "" && !strings.EqualFold(payCurrency, invCurrency) {
		return p.wrapError("CreatePayment", fmt.Errorf("%w: payment currency %s does not match invoice %s currency %s",
			ErrInvalidInput, input.Currency, input.InvoiceNo, invoice.Currency))
	}

	req := smartaccounts.CreatePaymentRequest{
		Date:        saFormatDate(input.PaymentDate),
		PartnerType: smartaccounts.PartnerClient,
		ClientID:    invoice.ClientID,
		AccountType: smartaccounts.AccountBank,
		AccountName: input.BankID,
		Currency:    input.Currency,
		Amount:      input.Amount,
		Rows: []smartaccounts.PaymentRow{{
			Type:   smartaccounts.RowClientInvoice,
			ID:     invoice.ID,
			Amount: input.Amount,
		}},
	}

	_, err = p.client.CreatePayment(ctx, req)
	return p.wrapError("CreatePayment", err)
}

func (p *smartProvider) ListPayments(ctx context.Context, input ListPaymentsInput) ([]Payment, error) {
	items, _, err := p.client.ListPayments(ctx, smartaccounts.ListPaymentsParams{
		DateFrom:  saFormatDate(input.PeriodStart),
		DateTo:    saFormatDate(input.PeriodEnd),
		FetchRows: true,
	})
	if err != nil {
		return nil, p.wrapError("ListPayments", err)
	}
	payments := make([]Payment, len(items))
	for i, item := range items {
		payments[i] = mapSAPayment(item)
	}
	return payments, nil
}

func (p *smartProvider) DeletePayment(ctx context.Context, id string) error {
	return p.wrapError("DeletePayment", p.client.DeletePayment(ctx, id))
}

// --- Items ---

func (p *smartProvider) CreateItem(ctx context.Context, input CreateItemInput) (*Item, error) {
	req := smartaccounts.ArticleItem{
		Code:            input.Code,
		Description:     input.Description,
		Type:            itemTypeToSA(input.Type),
		Unit:            input.UnitOfMeasure,
		VatPc:           input.TaxID,
		ActiveSales:     true,
		ActivePurchase:  true,
		PriceSales:      input.SalesPrice,
		AccountSales:    input.SalesAccountCode,
		AccountPurchase: input.PurchaseAccountCode,
	}
	resp, err := p.client.CreateArticle(ctx, req)
	if err != nil {
		return nil, p.wrapError("CreateItem", err)
	}
	return &Item{
		ID:            resp.Code,
		Code:          resp.Code,
		Name:          input.Description,
		Description:   input.Description,
		Type:          input.Type,
		UnitOfMeasure: input.UnitOfMeasure,
		SalesPrice:    input.SalesPrice,
		TaxID:         input.TaxID,
	}, nil
}

func (p *smartProvider) ListItems(ctx context.Context, input ListItemsInput) ([]Item, error) {
	items, err := p.client.ListArticles(ctx, smartaccounts.ListArticlesParams{
		SearchString: input.Description,
		Code:         input.Code,
	})
	if err != nil {
		return nil, p.wrapError("ListItems", err)
	}
	result := make([]Item, len(items))
	for i, a := range items {
		result[i] = mapSAArticle(a)
	}
	return result, nil
}

// UpdateItem fetches the existing article, applies changes, and sends the full
// object (SmartAccounts edit requires all fields).
func (p *smartProvider) UpdateItem(ctx context.Context, input UpdateItemInput) error {
	existing, err := p.client.ListArticles(ctx, smartaccounts.ListArticlesParams{Code: input.ID})
	if err != nil {
		return p.wrapError("UpdateItem", err)
	}
	if len(existing) == 0 {
		return &ProviderError{Provider: "smartaccounts", Op: "UpdateItem", Err: ErrNotFound}
	}
	art := existing[0]
	if input.Code != nil {
		art.Code = *input.Code
	}
	if input.Description != nil {
		art.Description = *input.Description
	}
	if input.SalesPrice != nil {
		art.PriceSales = *input.SalesPrice
	}
	if input.TaxID != nil {
		art.VatPc = *input.TaxID
	}
	if input.SalesAccountCode != nil {
		art.AccountSales = *input.SalesAccountCode
	}
	return p.wrapError("UpdateItem", p.client.EditArticle(ctx, art))
}

// --- Purchases ---

func (p *smartProvider) CreatePurchase(ctx context.Context, input CreatePurchaseInput) (*PurchaseInvoice, error) {
	vendorID, err := p.resolveVendorID(ctx, input.VendorID, input.VendorName, input.VendorRegNo)
	if err != nil {
		return nil, p.wrapError("CreatePurchase", err)
	}

	req := smartaccounts.CreateVendorInvoiceRequest{
		VendorID:        vendorID,
		Date:            saFormatDate(input.DocDate),
		DueDate:         saFormatDate(input.DueDate),
		InvoiceNumber:   input.BillNo,
		ReferenceNumber: input.RefNo,
		Currency:        input.Currency,
		IsCalculateVat:  true, // let SmartAccounts compute VAT from row vatPc
		Comment:         input.Comment,
		Rows:            buildSARows(input.Lines),
	}

	resp, err := p.client.CreateVendorInvoice(ctx, req)
	if err != nil {
		return nil, p.wrapError("CreatePurchase", err)
	}

	return &PurchaseInvoice{
		ID:          resp.InvoiceID,
		Number:      string(resp.InvoiceNumber),
		VendorName:  input.VendorName,
		VendorID:    resp.VendorID,
		DocDate:     input.DocDate,
		DueDate:     input.DueDate,
		TotalAmount: resp.TotalAmount,
		TaxAmount:   resp.VatAmount,
		Currency:    input.Currency,
		ReferenceNo: string(resp.ReferenceNumber),
		Status:      InvoiceStatusUnpaid,
	}, nil
}

func (p *smartProvider) GetPurchase(ctx context.Context, id string) (*PurchaseInvoice, error) {
	item, err := p.client.GetVendorInvoice(ctx, id)
	if err != nil {
		return nil, p.wrapError("GetPurchase", err)
	}
	pi := mapSAVendorInvoice(*item)
	return &pi, nil
}

func (p *smartProvider) ListPurchases(ctx context.Context, input ListPurchasesInput) ([]PurchaseInvoice, error) {
	items, err := p.client.ListVendorInvoices(ctx, smartaccounts.ListVendorInvoicesParams{
		DateFrom: saFormatDate(input.PeriodStart),
		DateTo:   saFormatDate(input.PeriodEnd),
	})
	if err != nil {
		return nil, p.wrapError("ListPurchases", err)
	}
	purchases := make([]PurchaseInvoice, len(items))
	for i, item := range items {
		purchases[i] = mapSAVendorInvoice(item)
	}
	return purchases, nil
}

func (p *smartProvider) DeletePurchase(ctx context.Context, id string) error {
	return p.wrapError("DeletePurchase", p.client.DeleteVendorInvoice(ctx, id))
}

// --- Reference data ---

func (p *smartProvider) ListTaxes(ctx context.Context) ([]Tax, error) {
	items, err := p.client.ListVatPcs(ctx)
	if err != nil {
		return nil, p.wrapError("ListTaxes", err)
	}
	taxes := make([]Tax, len(items))
	for i, v := range items {
		name := v.DescriptionEn
		if name == "" {
			name = v.DescriptionEt
		}
		taxes[i] = Tax{ID: v.VatPc, Code: v.VatPc, Name: name, Pct: v.Pc}
	}
	return taxes, nil
}

func (p *smartProvider) ListAccounts(ctx context.Context) ([]Account, error) {
	items, err := p.client.ListAccounts(ctx)
	if err != nil {
		return nil, p.wrapError("ListAccounts", err)
	}
	accounts := make([]Account, len(items))
	for i, a := range items {
		name := a.DescriptionEn
		if name == "" {
			name = a.DescriptionEt
		}
		accounts[i] = Account{ID: a.ID, Code: a.Code, Name: name, Active: true}
	}
	return accounts, nil
}

func (p *smartProvider) ListBanks(ctx context.Context) ([]Bank, error) {
	items, err := p.client.ListBankAccounts(ctx)
	if err != nil {
		return nil, p.wrapError("ListBanks", err)
	}
	banks := make([]Bank, len(items))
	for i, b := range items {
		banks[i] = Bank{
			ID:           b.Name, // SmartAccounts keys bank accounts by name
			Name:         b.Name,
			IBAN:         b.IBAN,
			AccountCode:  b.Account,
			CurrencyCode: b.Currency,
		}
	}
	return banks, nil
}

// ListPaymentTerms is a no-op for SmartAccounts: invoices use a numeric
// due-date-in-days field, not a payment-term code register.
func (p *smartProvider) ListPaymentTerms(_ context.Context) ([]PaymentTerm, error) {
	return nil, nil
}

func (p *smartProvider) ListDimensions(ctx context.Context) (*DimensionList, error) {
	objects, err := p.client.ListObjects(ctx)
	if err != nil {
		return nil, p.wrapError("ListDimensions", err)
	}
	// SmartAccounts has a single "objects" register; expose it as Projects.
	result := &DimensionList{Projects: make([]Dimension, len(objects))}
	for i, o := range objects {
		result.Projects[i] = Dimension{Code: o.Code, Name: o.Name}
	}
	return result, nil
}

// --- Reports ---

// CustomerDebts derives outstanding debts from unpaid (or overdue) client
// invoices. SmartAccounts has no dedicated debt report, and clientinvoices:get
// cannot filter by client name, so when customerName is given we filter the
// result client-side.
func (p *smartProvider) CustomerDebts(ctx context.Context, customerName string, overdueDays *int) ([]CustomerDebt, error) {
	status := "unpaid"
	if overdueDays != nil {
		status = "overdue"
	}
	items, _, err := p.client.ListInvoices(ctx, smartaccounts.ListInvoicesParams{PaymentStatus: status})
	if err != nil {
		return nil, p.wrapError("CustomerDebts", err)
	}

	want := strings.ToLower(strings.TrimSpace(customerName))
	var debts []CustomerDebt
	for _, item := range items {
		name := ""
		if item.Client != nil {
			name = item.Client.Name
		}
		if want != "" && strings.ToLower(strings.TrimSpace(name)) != want {
			continue
		}
		debts = append(debts, CustomerDebt{
			CustomerName: name,
			CustomerID:   item.ClientID,
			DocType:      "CLIENT_INVOICE",
			DocDate:      saParseDate(item.Date),
			DocNo:        string(item.InvoiceNumber),
			DueDate:      saParseDate(item.DueDate),
			TotalAmount:  item.TotalAmount,
			PaidAmount:   saPaidAmount(item.TotalAmount, item.OutstandingAmount),
			UnpaidAmount: item.OutstandingAmount,
			Currency:     item.Currency,
		})
	}
	return debts, nil
}

// --- Sync (incremental, by modify date) ---

func (p *smartProvider) ListInvoicesSince(ctx context.Context, since, until time.Time) ([]Invoice, error) {
	items, _, err := p.client.ListInvoices(ctx, smartaccounts.ListInvoicesParams{
		DateFrom:  saFormatDate(since),
		DateTo:    saFormatDate(until),
		DateType:  "modifydate",
		FetchRows: true,
	})
	if err != nil {
		return nil, p.wrapError("ListInvoicesSince", err)
	}
	// NOTE: the modifydate query also returns IDs deleted since `since` (the
	// `deleted` array). The Provider interface returns only []Invoice, so
	// deletions are not propagated yet — surfacing them needs an interface
	// change (see plan, follow-up).
	return mapSAInvoices(items), nil
}

func (p *smartProvider) ListPaymentsSince(ctx context.Context, since, until time.Time) ([]Payment, error) {
	items, _, err := p.client.ListPayments(ctx, smartaccounts.ListPaymentsParams{
		DateFrom:  saFormatDate(since),
		DateTo:    saFormatDate(until),
		DateType:  "modifydate",
		FetchRows: true,
	})
	if err != nil {
		return nil, p.wrapError("ListPaymentsSince", err)
	}
	payments := make([]Payment, len(items))
	for i, item := range items {
		payments[i] = mapSAPayment(item)
	}
	return payments, nil
}

// --- Resolution helpers ---

// resolveClientID returns the SmartAccounts client ID for an invoice/payment.
// If an explicit ID is given it is used; otherwise the client is looked up by
// registry code or name (SmartAccounts cannot create a client inline on an
// invoice the way Merit can).
func (p *smartProvider) resolveClientID(ctx context.Context, id, name, regCode string) (string, error) {
	if id != "" {
		return id, nil
	}
	query := regCode
	if query == "" {
		query = name
	}
	if query == "" {
		return "", fmt.Errorf("%w: SmartAccounts requires an existing client id or name", ErrInvalidInput)
	}
	clients, err := p.client.ListClients(ctx, smartaccounts.ListClientsParams{NameOrRegCode: query})
	if err != nil {
		return "", err
	}
	// Require an exact match on registry code or (case-insensitive) name.
	// nameOrRegCode is a fuzzy server-side filter, so a "use it if there's
	// exactly one result" fallback would silently book against the wrong legal
	// entity (e.g. "Acme" matching only "Acme Holdings International").
	for _, c := range clients {
		if regCode != "" && c.RegCode == regCode {
			return c.ID, nil
		}
		if name != "" && strings.EqualFold(strings.TrimSpace(c.Name), strings.TrimSpace(name)) {
			return c.ID, nil
		}
	}
	return "", fmt.Errorf("%w: no SmartAccounts client exactly matched %q", ErrNotFound, query)
}

func (p *smartProvider) resolveVendorID(ctx context.Context, id, name, regCode string) (string, error) {
	if id != "" {
		return id, nil
	}
	query := regCode
	if query == "" {
		query = name
	}
	if query == "" {
		return "", fmt.Errorf("%w: SmartAccounts requires an existing vendor id or name", ErrInvalidInput)
	}
	vendors, err := p.client.ListVendors(ctx, smartaccounts.ListVendorsParams{NameOrRegCode: query})
	if err != nil {
		return "", err
	}
	// Exact match only — see resolveClientID for why the single-result fallback
	// is unsafe.
	for _, v := range vendors {
		if regCode != "" && v.RegCode == regCode {
			return v.ID, nil
		}
		if name != "" && strings.EqualFold(strings.TrimSpace(v.Name), strings.TrimSpace(name)) {
			return v.ID, nil
		}
	}
	return "", fmt.Errorf("%w: no SmartAccounts vendor exactly matched %q", ErrNotFound, query)
}

// --- Mapping helpers ---

func mapSAInvoices(items []smartaccounts.InvoiceItem) []Invoice {
	out := make([]Invoice, len(items))
	for i, item := range items {
		out[i] = mapSAInvoice(item)
	}
	return out
}

func mapSAInvoice(item smartaccounts.InvoiceItem) Invoice {
	status, paid := deriveSAStatus(item.TotalAmount, item.OutstandingAmount)
	inv := Invoice{
		ID:          item.ID,
		Number:      string(item.InvoiceNumber),
		CustomerID:  item.ClientID,
		DocDate:     saParseDate(item.Date),
		DueDate:     saParseDate(item.DueDate),
		TotalAmount: item.TotalAmount,
		TaxAmount:   item.VatAmount,
		PaidAmount:  saPaidAmount(item.TotalAmount, item.OutstandingAmount),
		Currency:    item.Currency,
		Paid:        paid,
		Status:      status,
		ReferenceNo: string(item.ReferenceNumber),
	}
	if item.Client != nil {
		inv.CustomerName = item.Client.Name
	}
	inv.Lines = make([]InvoiceLine, len(item.Rows))
	for i, r := range item.Rows {
		inv.Lines[i] = InvoiceLine{
			Description:   r.Description,
			Quantity:      r.Quantity,
			UnitPrice:     r.Price,
			TaxID:         r.VatPc,
			TaxPct:        r.Vat,
			AmountInclVat: r.Sum,
			AccountCode:   r.AccountSales,
		}
	}
	return inv
}

func mapSAVendorInvoice(item smartaccounts.VendorInvoiceItem) PurchaseInvoice {
	status, paid := deriveSAStatus(item.TotalAmount, item.OutstandingAmount)
	pi := PurchaseInvoice{
		ID:          item.ID,
		Number:      string(item.InvoiceNumber),
		VendorID:    item.VendorID,
		DocDate:     saParseDate(item.Date),
		DueDate:     saParseDate(item.DueDate),
		TotalAmount: item.TotalAmount,
		TaxAmount:   item.VatAmount,
		PaidAmount:  saPaidAmount(item.TotalAmount, item.OutstandingAmount),
		Currency:    item.Currency,
		Paid:        paid,
		Status:      status,
		ReferenceNo: string(item.ReferenceNumber),
	}
	if item.Vendor != nil {
		pi.VendorName = item.Vendor.Name
	}
	return pi
}

func mapSAClient(item smartaccounts.ClientItem) Customer {
	c := Customer{
		ID:          item.ID,
		Name:        item.Name,
		RegNo:       item.RegCode,
		VATRegNo:    item.VatNumber,
		Email:       contactValue(item.Contacts, smartaccounts.ContactEmail),
		Phone:       contactValue(item.Contacts, smartaccounts.ContactPhone),
		PaymentDays: item.InvoiceDueDate,
		RefNoBase:   item.ReferenceNumber,
	}
	if item.Address != nil {
		c.Address = item.Address.Address1
		c.City = item.Address.City
		c.County = item.Address.County
		c.PostalCode = item.Address.PostalCode
		c.CountryCode = item.Address.Country
	}
	return c
}

func mapSAPayment(item smartaccounts.PaymentItem) Payment {
	links := make([]PaymentInvoiceLink, 0, len(item.Rows))
	for _, r := range item.Rows {
		if r.Type == smartaccounts.RowClientInvoice || r.Type == smartaccounts.RowVendorInvoice {
			links = append(links, PaymentInvoiceLink{InvoiceID: r.ID, Amount: r.Amount})
		}
	}

	pay := Payment{
		ID:               item.ID,
		DocumentNo:       string(item.Number),
		DocumentDate:     saParseDate(item.Date),
		Amount:           item.Amount,
		Currency:         item.Currency,
		Direction:        saPaymentDirection(item.PartnerType),
		InvoiceLinks:     links,
		ExternalBankName: item.AccountName,
		ExternalPayMode:  item.AccountName,
	}
	if item.Client != nil {
		pay.CounterPartID = item.Client.ID
		pay.CounterPartName = item.Client.Name
	} else if item.Vendor != nil {
		pay.CounterPartID = item.Vendor.ID
		pay.CounterPartName = item.Vendor.Name
	}
	return pay
}

func mapSAArticle(a smartaccounts.ArticleItem) Item {
	return Item{
		ID:            a.Code,
		Code:          a.Code,
		Name:          a.Description,
		Description:   a.Description,
		Type:          itemTypeFromSA(a.Type),
		UnitOfMeasure: a.Unit,
		SalesPrice:    a.PriceSales,
		TaxID:         a.VatPc,
	}
}

// deriveSAStatus maps total/outstanding to a status, sign-aware so credit notes
// (negative total, outstanding running from total toward zero as it is offset)
// are not misreported as paid. For a normal invoice, "settled" means outstanding
// reached zero (or went negative on overpayment); for a credit note it means
// outstanding climbed back to zero.
// saPaidAmount is the magnitude paid/offset so far. Using the absolute value
// keeps it correct for credit notes, whose total and outstanding are negative
// (a fully-offset credit note would otherwise report a negative paid amount).
func saPaidAmount(total, outstanding decimal.Decimal) decimal.Decimal {
	return total.Sub(outstanding).Abs()
}

func deriveSAStatus(total, outstanding decimal.Decimal) (InvoiceStatus, bool) {
	if total.IsNegative() {
		if outstanding.GreaterThanOrEqual(decimal.Zero) {
			return InvoiceStatusPaid, true // fully offset
		}
		if outstanding.GreaterThan(total) {
			return InvoiceStatusPartial, false
		}
		return InvoiceStatusUnpaid, false
	}
	if outstanding.LessThanOrEqual(decimal.Zero) {
		return InvoiceStatusPaid, true
	}
	if outstanding.LessThan(total) {
		return InvoiceStatusPartial, false
	}
	return InvoiceStatusUnpaid, false
}

func saPaymentDirection(partnerType string) PaymentDirection {
	if partnerType == smartaccounts.PartnerVendor {
		return PaymentDirectionVendor
	}
	return PaymentDirectionCustomer
}

func contactValue(contacts []smartaccounts.Contact, typ string) string {
	for _, c := range contacts {
		if c.Type == typ {
			return c.Value
		}
	}
	return ""
}

func buildSAContacts(email, phone string) []smartaccounts.Contact {
	var cs []smartaccounts.Contact
	if email != "" {
		cs = append(cs, smartaccounts.Contact{Type: smartaccounts.ContactEmail, Value: email})
	}
	if phone != "" {
		cs = append(cs, smartaccounts.Contact{Type: smartaccounts.ContactPhone, Value: phone})
	}
	return cs
}

func buildSAAddress(address, city, county, postalCode, country string) *smartaccounts.Address {
	if address == "" && city == "" && county == "" && postalCode == "" && country == "" {
		return nil
	}
	return &smartaccounts.Address{
		Country:    country,
		County:     county,
		City:       city,
		Address1:   address,
		PostalCode: postalCode,
	}
}

func itemTypeToSA(t ItemType) string {
	switch t {
	case ItemTypeStock:
		return smartaccounts.ArticleWarehouse
	case ItemTypeService:
		return smartaccounts.ArticleService
	default:
		return smartaccounts.ArticleProduct
	}
}

func itemTypeFromSA(t string) ItemType {
	switch t {
	case smartaccounts.ArticleWarehouse:
		return ItemTypeStock
	case smartaccounts.ArticleService:
		return ItemTypeService
	default:
		return ItemTypeItem
	}
}

// wrapError converts SmartAccounts APIError status codes into sentinel errors.
func (p *smartProvider) wrapError(op string, err error) error {
	if err == nil {
		return nil
	}
	var apiErr *smartaccounts.APIError
	if errors.As(err, &apiErr) {
		var sentinel error
		switch apiErr.StatusCode {
		case 401, 403:
			sentinel = ErrAuthFailed
		case 404:
			sentinel = ErrNotFound
		case 429, 503:
			sentinel = ErrRateLimit
		default:
			sentinel = err
		}
		return &ProviderError{Provider: "smartaccounts", Op: op, Err: sentinel}
	}
	return &ProviderError{Provider: "smartaccounts", Op: op, Err: err}
}
