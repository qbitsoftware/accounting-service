package accounting

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/qbitsoftware/accounting-service/smartaccounts"
)

// SmartAccounts receipt-level operations (v0.15.0). These are new code paths
// beside CreatePayment / ListPaymentsSince / mapSAPayment, which stay exactly as
// they were: a club that is not moved onto receipt-level behaviour never reaches
// anything in this file.

var (
	_ ReceiptCreator       = (*smartProvider)(nil)
	_ InvoiceReceiptLister = (*smartProvider)(nil)
	_ ReceiptGetter        = (*smartProvider)(nil)
	_ PaymentChangeLister  = (*smartProvider)(nil)
	_ CashAccountLister    = (*smartProvider)(nil)
	_ NettingBankLister    = (*smartProvider)(nil)
	_ BudgetReporter       = (*smartProvider)(nil)
)

var (
	// ErrProviderInvoiceMismatch: the invoice found by number is not the one
	// the caller expected (ExpectInvoiceID). Nothing was written.
	ErrProviderInvoiceMismatch = errors.New("provider invoice does not match the expected id")
	// ErrPartnerFilterIgnored: a per-customer read returned another customer's
	// receipts, so it answers nothing about this customer.
	ErrPartnerFilterIgnored = smartaccounts.ErrPartnerFilterIgnored
	// ErrTooManyPages: a capped read was still unfinished at its cap.
	ErrTooManyPages = smartaccounts.ErrTooManyPages
	// ErrIDFilterIgnored: a by-id read answered other payments, so it says
	// nothing about whether the asked-for receipt exists.
	ErrIDFilterIgnored = smartaccounts.ErrIDFilterIgnored
	// ErrUnreadablePayment: a per-customer read met a receipt it could not
	// decode, so whose it is — and whether the list is complete — is unknown.
	ErrUnreadablePayment = smartaccounts.ErrUnreadablePayment
)

// CreateReceipt books a receipt the way CreatePayment does, and additionally:
// returns SmartAccounts' paymentId, writes Document/Comment when set, books into
// a cash desk for PaymentAccountCash, skips the number lookup when the caller
// already holds the invoice (ResolvedInvoice), and refuses before writing when
// the invoice found by number is not the expected one (ExpectInvoiceID).
func (p *smartProvider) CreateReceipt(ctx context.Context, input CreatePaymentInput) (*CreatedReceipt, error) {
	accountType, err := saAccountType(input.AccountKind)
	if err != nil {
		return nil, p.wrapError("CreateReceipt", err)
	}

	var invoiceID, clientID, invCurrency string
	if ri := input.ResolvedInvoice; ri != nil {
		if ri.ID == "" || ri.CustomerID == "" {
			return nil, p.wrapError("CreateReceipt", fmt.Errorf("%w: ResolvedInvoice needs the provider's invoice and customer id", ErrInvalidInput))
		}
		if input.ExpectInvoiceID != "" && input.ExpectInvoiceID != ri.ID {
			return nil, p.wrapError("CreateReceipt", fmt.Errorf("%w: resolved invoice %s, expected %s", ErrProviderInvoiceMismatch, ri.ID, input.ExpectInvoiceID))
		}
		invoiceID, clientID, invCurrency = ri.ID, ri.CustomerID, ri.Currency
	} else {
		inv, err := p.client.FindInvoiceByNumber(ctx, input.InvoiceNo)
		if err != nil {
			return nil, p.wrapError("CreateReceipt", err)
		}
		if input.ExpectInvoiceID != "" && inv.ID != input.ExpectInvoiceID {
			return nil, p.wrapError("CreateReceipt", fmt.Errorf("%w: invoice number %s is %s in SmartAccounts, expected %s",
				ErrProviderInvoiceMismatch, input.InvoiceNo, inv.ID, input.ExpectInvoiceID))
		}
		invoiceID, clientID, invCurrency = inv.ID, inv.ClientID, inv.Currency
	}

	// Same currency guard as CreatePayment.
	payCurrency := strings.TrimSpace(input.Currency)
	invCurrency = strings.TrimSpace(invCurrency)
	if payCurrency != "" && invCurrency != "" && !strings.EqualFold(payCurrency, invCurrency) {
		return nil, p.wrapError("CreateReceipt", fmt.Errorf("%w: payment currency %s does not match invoice %s currency %s",
			ErrInvalidInput, input.Currency, input.InvoiceNo, invCurrency))
	}

	resp, err := p.client.CreatePayment(ctx, buildSAReceiptRequest(input, accountType, invoiceID, clientID))
	if err != nil {
		return nil, p.wrapError("CreateReceipt", err)
	}
	out := &CreatedReceipt{
		ID:         resp.PaymentID,
		Number:     string(resp.Number),
		Amount:     resp.Amount,
		InvoiceID:  invoiceID,
		CustomerID: clientID,
	}
	if out.Amount.IsZero() {
		out.Amount = input.Amount
	}
	return out, nil
}

func saAccountType(kind PaymentAccountKind) (string, error) {
	switch kind {
	case PaymentAccountDefault:
		return smartaccounts.AccountBank, nil
	case PaymentAccountCash:
		return smartaccounts.AccountCash, nil
	default:
		return "", fmt.Errorf("%w: unknown payment account kind %q", ErrInvalidInput, kind)
	}
}

// buildSAReceiptRequest is CreatePayment's request plus the receipt fields. It
// is its own builder so CreatePayment's stays byte-for-byte what it was.
func buildSAReceiptRequest(input CreatePaymentInput, accountType, invoiceID, clientID string) smartaccounts.CreatePaymentRequest {
	return smartaccounts.CreatePaymentRequest{
		Date:        saFormatDate(input.PaymentDate),
		Document:    input.Document,
		PartnerType: smartaccounts.PartnerClient,
		ClientID:    clientID,
		AccountType: accountType,
		AccountName: input.BankID,
		Currency:    input.Currency,
		Amount:      input.Amount,
		Comment:     input.Comment,
		Rows: []smartaccounts.PaymentRow{{
			Type:   smartaccounts.RowClientInvoice,
			ID:     invoiceID,
			Amount: input.Amount,
		}},
	}
}

// ListPaymentChanges reads the modifydate payments feed (rows and comments
// included) and returns it both as receipts and in the ListPaymentsSince
// projection. Precise sends Since to the second in Tallinn time (a cursor), and
// Until as the Tallinn calendar date: a UTC date taken between 21:00 and 24:00
// UTC is the day before Tallinn's today, which would cut the newest changes out
// of the window. Without Precise both are formatted exactly as ListPaymentsSince
// formats them.
func (p *smartProvider) ListPaymentChanges(ctx context.Context, input PaymentChangesInput) (*PaymentChanges, error) {
	if input.Since.IsZero() {
		return nil, p.wrapError("ListPaymentChanges", fmt.Errorf("%w: Since is required", ErrInvalidInput))
	}
	params := smartaccounts.ListPaymentsParams{
		DateType:      "modifydate",
		FetchRows:     true,
		FetchComments: true,
	}
	if input.Precise {
		params.DateFromPrecise = input.Since
		params.DateTo = smartaccounts.TallinnDate(input.Until)
	} else {
		params.DateFrom = saFormatDate(input.Since)
		params.DateTo = saFormatDate(input.Until)
	}
	if input.ClientsOnly {
		params.PartnerType = smartaccounts.PartnerClient
	}
	items, deleted, complete, err := p.client.ListPaymentChanges(ctx, params)
	if err != nil {
		return nil, p.wrapError("ListPaymentChanges", err)
	}
	out := &PaymentChanges{
		Receipts:        make([]Receipt, 0, len(items)),
		Payments:        make([]Payment, 0, len(items)),
		Deleted:         deleted,
		DeletedComplete: complete,
	}
	for _, it := range items {
		out.Receipts = append(out.Receipts, mapSAReceipt(it))
		// The projection is mapSAPayment's, unchanged. A payment whose core
		// fields did not decode has none (the old read would have failed on it
		// outright); it is still in Receipts, carrying its DecodeError.
		if !it.Unreadable {
			out.Payments = append(out.Payments, mapSAPayment(it.PaymentItem))
		}
	}
	return out, nil
}

// GetReceipt reads one receipt by id. A receipt that no longer exists answers
// an error for which IsNotFound is true.
func (p *smartProvider) GetReceipt(ctx context.Context, id string) (*Receipt, error) {
	it, err := p.client.GetPayment(ctx, id)
	if err != nil {
		return nil, p.wrapError("GetReceipt", err)
	}
	r := mapSAReceipt(*it)
	return &r, nil
}

// ListInvoiceReceipts reads one customer's receipts (at most MaxPages pages,
// default 5) and, when InvoiceID is set, keeps those with a row on it. It fails
// with ErrPartnerFilterIgnored or ErrTooManyPages rather than return a list that
// is not what was asked for.
func (p *smartProvider) ListInvoiceReceipts(ctx context.Context, input ListInvoiceReceiptsInput) ([]Receipt, error) {
	if input.CustomerID == "" {
		return nil, p.wrapError("ListInvoiceReceipts", fmt.Errorf("%w: CustomerID is required", ErrInvalidInput))
	}
	items, err := p.client.ListCustomerPayments(ctx, input.CustomerID, input.Since, input.Until, input.MaxPages)
	if err != nil {
		return nil, p.wrapError("ListInvoiceReceipts", err)
	}
	out := make([]Receipt, 0, len(items))
	for _, it := range items {
		r := mapSAReceipt(it)
		if input.InvoiceID != "" && !r.hasRowOn(input.InvoiceID) {
			continue
		}
		out = append(out, r)
	}
	return out, nil
}

func (r Receipt) hasRowOn(docID string) bool {
	for _, row := range r.Rows {
		if row.DocID == docID {
			return true
		}
	}
	return false
}

// ListCashAccounts returns the company's cash desks as Banks.
func (p *smartProvider) ListCashAccounts(ctx context.Context) ([]Bank, error) {
	items, err := p.client.ListCashAccounts(ctx)
	if err != nil {
		return nil, p.wrapError("ListCashAccounts", err)
	}
	out := make([]Bank, len(items))
	for i, c := range items {
		out[i] = Bank{ID: c.Name, Name: c.Name, AccountCode: c.Account, CurrencyCode: c.Currency}
	}
	return out, nil
}

// ListNettingBanks names the bank accounts the company marks forNetting.
func (p *smartProvider) ListNettingBanks(ctx context.Context) ([]string, error) {
	names, err := p.client.ListNettingBankNames(ctx)
	if err != nil {
		return nil, p.wrapError("ListNettingBanks", err)
	}
	return names, nil
}

// BudgetUsed is this process's request count for the company in the trailing
// 24 hours — a floor (see smartaccounts.Client.BudgetUsed).
func (p *smartProvider) BudgetUsed() int { return p.client.BudgetUsed() }

// mapSAReceipt keeps every row apart and reports the raw facts a caller needs
// to classify them: the parked advance, the advances drawn from, the document,
// the comment and anything that could not be read.
func mapSAReceipt(it smartaccounts.PaymentChangeItem) Receipt {
	r := Receipt{
		ID:          it.ID,
		Number:      string(it.Number),
		Document:    it.Document,
		Comment:     it.Comment,
		OrderNumber: it.OrderNumberString(),
		Date:        saParseDate(it.Date),
		PartnerType: it.PartnerType,
		AccountType: it.AccountType,
		AccountName: it.AccountName,
		Currency:    it.Currency,
		Amount:      it.Amount,
		CustomerID:  it.ClientID,
		HasExtras:   it.HasExtras(),
		DecodeError: it.DecodeError,
	}
	if it.Client != nil {
		if it.Client.ID != "" {
			r.CustomerID = it.Client.ID
		}
		r.CustomerName = it.Client.Name
	}
	if it.Prepayment != nil {
		r.AdvanceAmount = it.Prepayment.Amount
	}
	r.Rows = make([]ReceiptRow, len(it.Rows))
	for i, row := range it.Rows {
		r.Rows[i] = ReceiptRow{Type: row.Type, DocID: row.ID, Amount: row.Amount, Description: row.Description}
		if row.Type == smartaccounts.RowPrepayment {
			r.AdvanceSourceIDs = append(r.AdvanceSourceIDs, row.ID)
		}
	}
	return r
}
