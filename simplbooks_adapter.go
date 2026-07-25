package accounting

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/qbitsoftware/accounting-service/simplbooks"
	"github.com/shopspring/decimal"
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

	// articleCache maps a klubio article code to the SimplBooks article it names,
	// so an invoice line can carry a readable code ("Liikmemaks") instead of an
	// opaque numeric id. Loaded lazily on first use and reused for the life of
	// the provider — klubio builds one client per sync run, so a batch costs a
	// single articles/list against the 60 req/min budget.
	articleMu     sync.Mutex
	articleByCode map[string]int  // lower(code) -> article id
	articleIDs    map[int]bool    // known ids, to validate a numeric code
	articleLoaded bool
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
	clientID := atoiOrZero(input.CustomerID)
	if clientID == 0 {
		return nil, p.wrapError("CreateInvoice", fmt.Errorf(
			"%w: CustomerID must be a numeric SimplBooks client_id (got %q) — SimplBooks has no inline customer on an invoice",
			ErrInvalidInput, input.CustomerID))
	}
	invoice := map[string]any{
		"row_sum_with_vat": false, // line prices are net; VAT is added per row
		"client_id":        clientID,
	}
	// Dates are only sent when set: formatSimplBooksDate returns "" for a zero
	// time, and an empty string is not "use the account default" — it either
	// 400s or stores 0000-00-00.
	putIf(invoice, "created", formatSimplBooksDate(input.DocDate))
	putIf(invoice, "transaction_date", formatSimplBooksDate(input.DocDate))
	putIf(invoice, "due", formatSimplBooksDate(input.DueDate))
	// SimplBooks tracks delivery on the document: an invoice created without a
	// `sent` date shows as unsent in its UI. klubio only pushes invoices it has
	// already mailed out, so carry the real send date across — otherwise every
	// synced invoice looks like it is still waiting to go out.
	putIf(invoice, "sent", formatSimplBooksDate(input.SentDate))
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
		tasks = append(tasks, map[string]any{"Task": simplbooksRow(line, p.resolveArticleID(ctx, line.Code))})
	}

	id, err := p.client.CreateInvoice(ctx, map[string]any{
		"Invoice": invoice,
		"Tasks":   tasks,
	})
	if err != nil {
		return nil, p.wrapError("CreateInvoice", err)
	}
	return p.getInvoiceEnsuringSent(ctx, strconv.Itoa(id), input.SentDate, "CreateInvoice")
}

// getInvoiceEnsuringSent reads a freshly created invoice back and, when a send
// date was asked for but did not stick, repairs the flag via invoices/sent.
//
// `sent` is a documented create field, but a create that quietly drops it would
// leave every synced invoice showing as unsent — the exact problem this is meant
// to fix — with no signal at the call site. The read-back happens anyway, so the
// check is free; the extra request is only spent when the field was ignored, and
// it stamps SimplBooks' current date rather than the original send date.
func (p *simplbooksProvider) getInvoiceEnsuringSent(ctx context.Context, id string, want time.Time, op string) (*Invoice, error) {
	inv, err := p.client.GetInvoice(ctx, id)
	if err != nil {
		return nil, p.wrapError(op, err)
	}
	if !want.IsZero() && parseSimplBooksDate(inv.Sent).IsZero() {
		if markErr := p.client.MarkInvoiceSent(ctx, id); markErr != nil {
			// Non-fatal: the document itself is correct, it just displays as
			// unsent. Failing the create here would have klubio retry and
			// duplicate the invoice, which is far worse than a wrong flag.
			slog.Warn("simplbooks: invoice created but could not be marked as sent",
				"invoice_id", id, "error", markErr)
		}
	}
	return mapSimplBooksInvoice(inv), nil
}

func (p *simplbooksProvider) GetInvoice(ctx context.Context, id string) (*Invoice, error) {
	inv, err := p.client.GetInvoice(ctx, id)
	if err != nil {
		return nil, p.wrapError("GetInvoice", err)
	}
	return mapSimplBooksInvoice(inv), nil
}

// MarkInvoiceSent implements InvoiceSentMarker. SimplBooks stamps its own
// current date — the endpoint takes no date argument — so this is the repair
// path for invoices pushed before the sent date was carried on create.
func (p *simplbooksProvider) MarkInvoiceSent(ctx context.Context, id string) error {
	return p.wrapError("MarkInvoiceSent", p.client.MarkInvoiceSent(ctx, id))
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
	p.fillPartialBalances(ctx, invoices)
	return invoices, nil
}

// simplbooksBalanceFetchLimit caps how many invoices fillPartialBalances will
// fetch individually, so a large back-window can't consume the whole 60 req/min
// budget. Anything beyond the cap keeps its list-derived figure.
const simplbooksBalanceFetchLimit = 300

// fillPartialBalances repairs PaidAmount for PARTIALLY paid invoices.
//
// invoices/list carries no `remnant` — not even when explicitly requested via
// `fields`, verified live — and a partial payment leaves `paid` at the
// "0000-00-00" sentinel. So a half-paid invoice comes out of the list looking
// completely unpaid, and klubio's payment pull (which acts on provider paid >
// local paid) silently never sees the money.
//
// Only invoices the list reports as UNSETTLED need this: a settled one already
// maps to PaidAmount == total, which is what invoices/get would say too. In a
// healthy club most invoices in the window are settled, so this stays a
// minority of rows.
func (p *simplbooksProvider) fillPartialBalances(ctx context.Context, invoices []Invoice) {
	fetched := 0
	for i := range invoices {
		if invoices[i].Paid || invoices[i].PaidAmount.IsPositive() {
			continue // already settled — the list figure is accurate
		}
		if fetched >= simplbooksBalanceFetchLimit {
			slog.Warn("simplbooks: partial-balance lookup capped; remaining invoices keep their list-derived paid amount",
				"limit", simplbooksBalanceFetchLimit, "unresolved", len(invoices)-i)
			return
		}
		full, err := p.client.GetInvoice(ctx, invoices[i].ID)
		if err != nil {
			continue // leave the list-derived figure rather than failing the whole list
		}
		fetched++
		invoices[i] = *mapSimplBooksInvoice(full)
	}
}

func (p *simplbooksProvider) DeleteInvoice(ctx context.Context, id string) error {
	return p.wrapError("DeleteInvoice", p.client.DeleteInvoice(ctx, id))
}

// simplbooksRefScanLimit bounds how many invoices FindInvoiceByRef will fetch
// individually. SimplBooks allows 60 requests/minute, so an unbounded scan of a
// club with thousands of invoices would stall for many minutes and starve every
// other call. Newest invoices are checked first, which is where a reference
// being looked up (a recent bank payment) almost always lives.
const simplbooksRefScanLimit = 250

// FindInvoiceByRef locates an invoice by its payment reference number.
//
// This is unavoidably expensive on SimplBooks: invoices/list neither returns
// `reference` (it is dropped even when requested via `fields`) nor filters on
// it (a `reference` filter is silently ignored and returns everything), so the
// reference is only visible via invoices/get on one invoice at a time. We list
// the IDs, then fetch newest-first until a reference matches, capped at
// simplbooksRefScanLimit fetches.
//
// Both API behaviours were verified against a live account. If SimplBooks ever
// exposes a reference filter, replace this whole scan with it.
func (p *simplbooksProvider) FindInvoiceByRef(ctx context.Context, refStr string) (*Invoice, error) {
	refStr = strings.TrimSpace(refStr)
	if refStr == "" {
		return nil, &ProviderError{Provider: "simplbooks", Op: "FindInvoiceByRef", Err: ErrNotFound}
	}
	items, err := p.client.ListAllInvoices(ctx, nil, []string{"id", "number"}, 200)
	if err != nil {
		return nil, p.wrapError("FindInvoiceByRef", err)
	}
	// Newest first — list order is not guaranteed, so sort by ID descending.
	sort.Slice(items, func(a, b int) bool { return items[a].ID > items[b].ID })
	if len(items) > simplbooksRefScanLimit {
		items = items[:simplbooksRefScanLimit]
	}
	for i := range items {
		full, err := p.client.GetInvoice(ctx, strconv.Itoa(items[i].ID))
		if err != nil {
			// A rate-limit stop must NOT be swallowed. This scan issues up to
			// simplbooksRefScanLimit sequential gets against a 60 req/min budget,
			// so 429s are expected on a busy account — and continuing past them
			// would return "not found" for an invoice that exists, silently
			// leaving a real bank payment unmatched. Abort and let the caller
			// retry instead.
			var apiErr *simplbooks.APIError
			if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusTooManyRequests {
				return nil, p.wrapError("FindInvoiceByRef", err)
			}
			// Any other single unreadable invoice must not abort the scan.
			continue
		}
		if strings.TrimSpace(full.Reference) == refStr {
			return mapSimplBooksInvoice(full), nil
		}
	}
	return nil, &ProviderError{Provider: "simplbooks", Op: "FindInvoiceByRef", Err: ErrNotFound}
}

// --- Customers ---

// simplbooksAddressPlaceholder stands in for a missing street address.
//
// SimplBooks REJECTS clients/create without address_street ("Kliendi või
// tarnija tänava aadress on kohustuslik", 400) — verified live; the OpenAPI
// spec declares no required fields, so this is only discoverable by trying.
// Merit and Excellent Books have no such rule, so klubio rosters routinely
// carry members with no address at all, and hard-failing every one of them
// would block the whole sync. A neutral placeholder keeps the sync moving and
// is obvious in the client card, so an accountant can see what needs filling.
const simplbooksAddressPlaceholder = "-"

// simplbooksDefaultUnit is the unit used when the caller supplies none; see
// CreateItem for why one is always required.
const simplbooksDefaultUnit = "tk"

func (p *simplbooksProvider) CreateCustomer(ctx context.Context, input CreateCustomerInput) (*Customer, error) {
	fields := map[string]any{"name": input.Name}
	address := strings.TrimSpace(input.Address)
	if address == "" {
		address = simplbooksAddressPlaceholder
	}
	fields["address_street"] = address
	putIf(fields, "reg_no", input.RegNo)
	putIf(fields, "vat_no", input.VATRegNo)
	putIf(fields, "e_mail", input.Email)
	putIf(fields, "phone", input.Phone)
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
	// SimplBooks REJECTS articles/create without a unit ("Artikli ühik peab olema
	// määratud vähemalt ühes keeles", 400) — verified live; the OpenAPI spec
	// declares no required fields. klubio's article registry has no unit field
	// at all, so default to the Estonian "tk" (pcs) rather than failing every
	// article push.
	unit := strings.TrimSpace(input.UnitOfMeasure)
	if unit == "" {
		unit = simplbooksDefaultUnit
	}
	fields["unit"] = unit
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
	p.invalidateArticleCache()
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
	putIfPtr(fields, "code", input.Code)
	putIfPtr(fields, "name", input.Description)
	if input.SalesPrice != nil {
		fields["price_per_unit"] = input.SalesPrice.String()
	}
	// articles/update REPLACES the record, so it enforces the same mandatory
	// unit as create — omitting it fails with "Artikli ühik peab olema määratud
	// vähemalt ühes keeles". UpdateItemInput carries no unit, so preserve
	// whatever the article already has (an accountant may have set something
	// other than the default) and only fall back when it has none.
	unit := simplbooksDefaultUnit
	if current, err := p.client.GetArticle(ctx, strconv.Itoa(id)); err == nil {
		if u := strings.TrimSpace(current.Unit); u != "" {
			unit = u
		}
	}
	fields["unit"] = unit
	// NOTE: TaxID and SalesAccountCode are not forwarded — both are read-only on
	// the Article schema, same as on create.
	err := p.client.UpdateArticle(ctx, id, fields)
	if err == nil {
		p.invalidateArticleCache() // a code change moves the code->id mapping
	}
	return p.wrapError("UpdateItem", err)
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
	putIf(invoice, "sent", formatSimplBooksDate(input.SentDate)) // see CreateInvoice
	if id := atoiOrZero(input.CustomerID); id != 0 {
		invoice["client_id"] = id
	}
	if input.Currency != "" {
		invoice["currency_name"] = input.Currency
	}
	if input.RefNo != "" {
		invoice["reference"] = input.RefNo
	}
	// Carry klubio's own credit-note number and comment across, exactly as
	// CreateInvoice does. Without these SimplBooks auto-numbers the document
	// (e.g. "26001") and an accountant has no way to tie it back to the
	// KRINV… number the member was sent.
	if input.InvoiceNo != "" {
		invoice["number"] = input.InvoiceNo
	}
	if input.Comment != "" {
		invoice["additional_info"] = input.Comment
	}
	if orig := atoiOrZero(input.OriginalInvoiceNo); orig != 0 {
		invoice["credit_invoice_for"] = orig
	}

	tasks := make([]map[string]any, 0, len(input.Lines))
	for _, line := range input.Lines {
		tasks = append(tasks, map[string]any{"Task": simplbooksRow(line, p.resolveArticleID(ctx, line.Code))})
	}

	id, err := p.client.CreateInvoice(ctx, map[string]any{
		"Invoice": invoice,
		"Tasks":   tasks,
	})
	if err != nil {
		return nil, p.wrapError("CreateCreditNote", err)
	}
	return p.getInvoiceEnsuringSent(ctx, strconv.Itoa(id), input.SentDate, "CreateCreditNote")
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
		// ID must be the numeric financial-account ID, not the human-readable
		// code: an invoice row references the account via income_account_id,
		// which simplbooksRow parses out of line.AccountCode. Returning the code
		// here (e.g. "4400") would be parsed as account ID 4400 and silently
		// post revenue to the wrong account — or none at all. Code stays the
		// display code so pickers can still show "4400 — Käibemaksuvaba käive".
		accounts = append(accounts, Account{
			ID:     strconv.Itoa(a.ID),
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

// loadArticles fills the code->id cache. Caller must hold articleMu.
func (p *simplbooksProvider) loadArticlesLocked(ctx context.Context) error {
	arts, err := p.client.ListAllArticles(ctx, nil, 1000)
	if err != nil {
		return err
	}
	byCode := make(map[string]int, len(arts))
	ids := make(map[int]bool, len(arts))
	for _, a := range arts {
		ids[a.ID] = true
		if c := strings.ToLower(strings.TrimSpace(a.Code)); c != "" {
			byCode[c] = a.ID
		}
	}
	p.articleByCode = byCode
	p.articleIDs = ids
	p.articleLoaded = true
	return nil
}

// resolveArticleID maps an invoice line's Code to a SimplBooks article id.
//
// Resolution order:
//  1. an article whose CODE matches (case-insensitive) — the normal path, so
//     klubio can keep human-readable codes like "Liikmemaks";
//  2. failing that, a numeric code is treated as an article id, but only if an
//     article with that id actually exists. Without that existence check a
//     klubio code like "1001" would silently link to whatever unrelated article
//     happened to hold id 1001, dragging its data onto the line.
//
// Returns 0 when nothing matches: the line then goes out as free text, which
// SimplBooks accepts. It must NOT fall back to sending the code as-is —
// SimplBooks validates Task.code against its register and rejects the whole
// invoice ("Artikli ID ja artikli kood ei ole vastavuses...").
//
// A miss on a warm cache triggers one refresh, so an article created after the
// cache loaded (e.g. by CreateItem earlier in the same run) is picked up.
func (p *simplbooksProvider) resolveArticleID(ctx context.Context, code string) int {
	code = strings.TrimSpace(code)
	if code == "" {
		return 0
	}
	p.articleMu.Lock()
	defer p.articleMu.Unlock()

	lookup := func() int {
		if id, ok := p.articleByCode[strings.ToLower(code)]; ok {
			return id
		}
		if n := atoiOrZero(code); n != 0 && p.articleIDs[n] {
			return n
		}
		return 0
	}

	if !p.articleLoaded {
		if err := p.loadArticlesLocked(ctx); err != nil {
			// Can't reach the register — fall back to a free-text line rather
			// than failing the invoice over a cosmetic link.
			return 0
		}
		return lookup()
	}
	if id := lookup(); id != 0 {
		return id
	}
	// Warm cache, no match: the article may have been created since. Refresh once.
	if err := p.loadArticlesLocked(ctx); err != nil {
		return 0
	}
	return lookup()
}

// invalidateArticleCache drops the cached register after a write, so a
// subsequent line referencing the new/renamed article resolves.
func (p *simplbooksProvider) invalidateArticleCache() {
	p.articleMu.Lock()
	p.articleByCode, p.articleIDs, p.articleLoaded = nil, nil, false
	p.articleMu.Unlock()
}

func simplbooksRow(line CreateInvoiceLineInput, articleID int) map[string]any {
	row := map[string]any{
		"amount":         line.Quantity.String(),
		"price_per_unit": line.UnitPrice.String(),
	}
	name := line.Description
	if name == "" {
		name = line.Code
	}
	row["name"] = name
	// article_id only — never Task.code, which SimplBooks validates against its
	// own register and rejects when it doesn't match (see resolveArticleID).
	// SimplBooks fills the code itself from the linked article.
	if articleID != 0 {
		row["article_id"] = articleID
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

	// SimplBooks exposes no paid *amount* — only a `paid` settlement date and,
	// on invoices/get, a `remnant` (open balance). Derive the amount:
	//
	//   remnant present (invoices/get) -> paid = total - remnant, which is the
	//     only way to see a partial payment.
	//   remnant absent (invoices/list) -> fall back to the settlement date:
	//     a set date means fully paid, otherwise nothing is paid. Partial
	//     payments are invisible in the list projection; callers needing exact
	//     figures must GetInvoice.
	//
	// Klubio's payment pull compares this against the local paid amount and
	// only writes when it is greater, so leaving it at zero silently disables
	// payment reconciliation entirely.
	paid := parseSimplBooksDate(inv.Paid)
	isPaid := !paid.IsZero()
	status := InvoiceStatusUnpaid
	var paidAmount decimal.Decimal
	switch {
	case inv.Remnant != nil:
		paidAmount = inv.TotalSum.Decimal.Sub(inv.Remnant.Decimal)
		if paidAmount.IsNegative() {
			paidAmount = decimal.Zero
		}
	case isPaid:
		paidAmount = inv.TotalSum.Decimal
	}
	switch {
	case isPaid || (paidAmount.GreaterThanOrEqual(inv.TotalSum.Decimal) && paidAmount.IsPositive()):
		isPaid = true
		status = InvoiceStatusPaid
	case paidAmount.IsPositive():
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
		PaidAmount:   paidAmount,
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
