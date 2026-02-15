package accounting

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/qbitsoftware/accounting-service/merit"
	"github.com/shopspring/decimal"
)

// meritProvider implements Provider using the Merit Aktiva API.
type meritProvider struct {
	client *merit.Client
}

func newMeritProvider(cfg Config) *meritProvider {
	apiURL := merit.EstoniaURL
	switch strings.ToLower(cfg.Region) {
	case "pl", "poland":
		apiURL = merit.PolandURL
	}

	return &meritProvider{
		client: merit.New(merit.Config{
			APIURL:     apiURL,
			APIID:      cfg.APIID,
			APIKey:     cfg.APIKey,
			HTTPClient: cfg.HTTPClient,
		}),
	}
}

func (p *meritProvider) TestConnection(ctx context.Context) error {
	_, err := p.client.ListTaxes(ctx)
	return p.wrapError("TestConnection", err)
}

// --- Invoices ---

func buildRowsAndTaxes(lines []CreateInvoiceLineInput) ([]merit.InvoiceRow, []merit.TaxAmountEntry) {
	rows := make([]merit.InvoiceRow, len(lines))
	taxAmounts := make(map[string]decimal.Decimal)

	for i, line := range lines {
		rows[i] = merit.InvoiceRow{
			Item: merit.ItemRef{
				Code:        line.Code,
				Description: line.Description,
				Type:        line.Type,
				UOMName:     line.UOMName,
			},
			Quantity:      line.Quantity,
			Price:         line.UnitPrice,
			TaxID:         line.TaxID,
			GLAccountCode: line.AccountCode,
		}
		amount := line.Quantity.Mul(line.UnitPrice)
		taxAmounts[line.TaxID] = taxAmounts[line.TaxID].Add(amount)
	}

	taxes := make([]merit.TaxAmountEntry, 0, len(taxAmounts))
	for taxID, amount := range taxAmounts {
		taxes = append(taxes, merit.TaxAmountEntry{
			TaxID:  taxID,
			Amount: amount,
		})
	}

	return rows, taxes
}

func (p *meritProvider) CreateInvoice(ctx context.Context, input CreateInvoiceInput) (*Invoice, error) {
	rows, taxes := buildRowsAndTaxes(input.Lines)

	req := merit.CreateInvoiceRequest{
		Customer: merit.CustomerRef{
			ID:          input.CustomerID,
			Name:        input.CustomerName,
			RegNo:       input.CustomerRegNo,
			Email:       input.CustomerEmail,
			Address:     input.CustomerAddress,
			CountryCode: input.CustomerCountryCode,
		},
		AccountingDoc: merit.DocInvoice,
		DocDate:       formatDate(input.DocDate),
		DueDate:       formatDate(input.DueDate),
		InvoiceNo:     input.InvoiceNo,
		RefNo:         input.RefNo,
		CurrencyCode:  input.Currency,
		InvoiceRow:    rows,
		TaxAmount:     taxes,
		Hcomment:      input.Comment,
		Fcomment:      input.FooterComment,
	}

	resp, err := p.client.CreateInvoice(ctx, req)
	if err != nil {
		return nil, p.wrapError("CreateInvoice", err)
	}

	return &Invoice{
		ID:          resp.InvoiceID,
		Number:      resp.InvoiceNo,
		CustomerName: input.CustomerName,
		CustomerID:  resp.CustomerID,
		DocDate:     input.DocDate,
		DueDate:     input.DueDate,
		Currency:    input.Currency,
		ReferenceNo: resp.RefNo,
		Status:      InvoiceStatusUnpaid,
	}, nil
}

func (p *meritProvider) GetInvoice(ctx context.Context, id string) (*Invoice, error) {
	detail, err := p.client.GetInvoice(ctx, merit.GetInvoiceParams{ID: id})
	if err != nil {
		return nil, p.wrapError("GetInvoice", err)
	}
	return mapInvoiceDetail(detail), nil
}

func (p *meritProvider) GetInvoicePDF(ctx context.Context, id string, deliveryNote bool) (*InvoicePDF, error) {
	result, err := p.client.GetInvoicePDF(ctx, merit.GetInvoicePDFParams{
		ID:        id,
		DelivNote: deliveryNote,
	})
	if err != nil {
		return nil, p.wrapError("GetInvoicePDF", err)
	}

	content, err := base64.StdEncoding.DecodeString(result.FileContent)
	if err != nil {
		return nil, p.wrapError("GetInvoicePDF", fmt.Errorf("decode pdf: %w", err))
	}

	return &InvoicePDF{
		FileName:    result.FileName,
		FileContent: content,
	}, nil
}

func (p *meritProvider) ListInvoices(ctx context.Context, input ListInvoicesInput) ([]Invoice, error) {
	items, err := p.client.ListInvoices(ctx, merit.ListInvoicesParams{
		PeriodStart: input.PeriodStart,
		PeriodEnd:   input.PeriodEnd,
	})
	if err != nil {
		return nil, p.wrapError("ListInvoices", err)
	}

	invoices := make([]Invoice, len(items))
	for i, item := range items {
		invoices[i] = mapInvoiceListItem(item)
	}
	return invoices, nil
}

func (p *meritProvider) DeleteInvoice(ctx context.Context, id string) error {
	err := p.client.DeleteInvoice(ctx, merit.DeleteInvoiceParams{ID: id})
	return p.wrapError("DeleteInvoice", err)
}

// --- Customers ---

func (p *meritProvider) CreateCustomer(ctx context.Context, input CreateCustomerInput) (*Customer, error) {
	req := merit.CreateCustomerRequest{
		Name:            input.Name,
		RegNo:           input.RegNo,
		VatRegNo:        input.VATRegNo,
		Email:           input.Email,
		PhoneNo:         input.Phone,
		Address:         input.Address,
		City:            input.City,
		County:          input.County,
		PostalCode:      input.PostalCode,
		CountryCode:     input.CountryCode,
		CurrencyCode:    input.Currency,
		PaymentDeadLine: input.PaymentDays,
		Contact:         input.Contact,
	}

	resp, err := p.client.CreateCustomer(ctx, req)
	if err != nil {
		return nil, p.wrapError("CreateCustomer", err)
	}

	return &Customer{
		ID:          resp.ID,
		Name:        resp.Name,
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

func (p *meritProvider) UpdateCustomer(ctx context.Context, input UpdateCustomerInput) error {
	req := merit.UpdateCustomerRequest{
		ID: input.ID,
	}
	if input.Name != nil {
		req.Name = *input.Name
	}
	if input.Email != nil {
		req.Email = *input.Email
	}
	if input.Phone != nil {
		req.PhoneNo = *input.Phone
	}
	if input.Address != nil {
		req.Address = *input.Address
	}
	if input.City != nil {
		req.City = *input.City
	}
	if input.PostalCode != nil {
		req.PostalCode = *input.PostalCode
	}
	if input.CountryCode != nil {
		req.CountryCode = *input.CountryCode
	}
	if input.RegNo != nil {
		req.RegNo = *input.RegNo
	}
	if input.VATRegNo != nil {
		req.VatRegNo = *input.VATRegNo
	}

	err := p.client.UpdateCustomer(ctx, req)
	return p.wrapError("UpdateCustomer", err)
}

func (p *meritProvider) ListCustomers(ctx context.Context, input ListCustomersInput) ([]Customer, error) {
	items, err := p.client.ListCustomers(ctx, merit.ListCustomersParams{})
	if err != nil {
		return nil, p.wrapError("ListCustomers", err)
	}

	customers := make([]Customer, len(items))
	for i, item := range items {
		customers[i] = mapCustomerListItem(item)
	}
	return customers, nil
}

func (p *meritProvider) FindCustomerByEmail(ctx context.Context, email string) (*Customer, error) {
	items, err := p.client.ListCustomers(ctx, merit.ListCustomersParams{})
	if err != nil {
		return nil, p.wrapError("FindCustomerByEmail", err)
	}

	email = strings.ToLower(strings.TrimSpace(email))
	for _, item := range items {
		if strings.ToLower(strings.TrimSpace(item.Email)) == email {
			c := mapCustomerListItem(item)
			return &c, nil
		}
	}

	return nil, &ProviderError{Provider: "merit", Op: "FindCustomerByEmail", Err: ErrNotFound}
}

// --- Payments ---

func (p *meritProvider) CreatePayment(ctx context.Context, input CreatePaymentInput) error {
	req := merit.CreatePaymentRequest{
		BankID:       input.BankID,
		CustomerName: input.CustomerName,
		InvoiceNo:    input.InvoiceNo,
		PaymentDate:  formatDate(input.PaymentDate),
		Amount:       input.Amount,
		CurrencyCode: input.Currency,
	}
	err := p.client.CreatePayment(ctx, req)
	return p.wrapError("CreatePayment", err)
}

func (p *meritProvider) ListPayments(ctx context.Context, input ListPaymentsInput) ([]Payment, error) {
	items, err := p.client.ListPayments(ctx, merit.ListPaymentsParams{
		PeriodStart: input.PeriodStart,
		PeriodEnd:   input.PeriodEnd,
	})
	if err != nil {
		return nil, p.wrapError("ListPayments", err)
	}

	payments := make([]Payment, len(items))
	for i, item := range items {
		payments[i] = mapPaymentListItem(item)
	}
	return payments, nil
}

func (p *meritProvider) DeletePayment(ctx context.Context, id string) error {
	err := p.client.DeletePayment(ctx, merit.DeletePaymentParams{ID: id})
	return p.wrapError("DeletePayment", err)
}

// --- Items ---

func (p *meritProvider) CreateItem(ctx context.Context, input CreateItemInput) (*Item, error) {
	req := merit.CreateItemRequest{
		Type:            mapItemTypeToMerit(input.Type),
		Usage:           merit.ItemUsageBoth,
		Code:            input.Code,
		Description:     input.Description,
		UOMName:         input.UnitOfMeasure,
		TaxID:           input.TaxID,
		SalesAccCode:    input.SalesAccountCode,
		PurchaseAccCode: input.PurchaseAccountCode,
	}

	results, err := p.client.CreateItems(ctx, []merit.CreateItemRequest{req})
	if err != nil {
		return nil, p.wrapError("CreateItem", err)
	}
	if len(results) == 0 {
		return nil, p.wrapError("CreateItem", errors.New("empty response"))
	}

	return &Item{
		ID:            results[0].ItemID,
		Code:          results[0].Code,
		Description:   input.Description,
		Type:          input.Type,
		UnitOfMeasure: input.UnitOfMeasure,
		SalesPrice:    input.SalesPrice,
		TaxID:         input.TaxID,
	}, nil
}

func (p *meritProvider) ListItems(ctx context.Context, input ListItemsInput) ([]Item, error) {
	params := merit.ListItemsParams{
		Code:        input.Code,
		Description: input.Description,
	}
	if input.Type != "" {
		t := mapItemTypeToMerit(input.Type)
		params.Type = &t
	}

	results, err := p.client.ListItems(ctx, params)
	if err != nil {
		return nil, p.wrapError("ListItems", err)
	}

	items := make([]Item, len(results))
	for i, r := range results {
		items[i] = mapItemListItem(r)
	}
	return items, nil
}

func (p *meritProvider) UpdateItem(ctx context.Context, input UpdateItemInput) error {
	req := merit.UpdateItemRequest{
		ID: input.ID,
	}
	if input.Code != nil {
		req.Code = *input.Code
	}
	if input.Description != nil {
		req.Description = *input.Description
	}
	if input.SalesPrice != nil {
		req.SalesPrice = *input.SalesPrice
	}
	if input.TaxID != nil {
		req.TaxID = *input.TaxID
	}

	err := p.client.UpdateItem(ctx, req)
	return p.wrapError("UpdateItem", err)
}

// --- Credit Notes ---

func (p *meritProvider) CreateCreditNote(ctx context.Context, input CreateCreditNoteInput) (*Invoice, error) {
	rows, taxes := buildRowsAndTaxes(input.Lines)

	req := merit.CreateInvoiceRequest{
		Customer: merit.CustomerRef{
			ID:          input.CustomerID,
			Name:        input.CustomerName,
			RegNo:       input.CustomerRegNo,
			Email:       input.CustomerEmail,
			Address:     input.CustomerAddress,
			CountryCode: input.CustomerCountryCode,
		},
		AccountingDoc: merit.DocCredit,
		DocDate:       formatDate(input.DocDate),
		DueDate:       formatDate(input.DueDate),
		InvoiceNo:     input.InvoiceNo,
		RefNo:         input.RefNo,
		CurrencyCode:  input.Currency,
		InvoiceRow:    rows,
		TaxAmount:     taxes,
		Hcomment:      input.Comment,
		Fcomment:      input.FooterComment,
	}

	resp, err := p.client.CreateInvoice(ctx, req)
	if err != nil {
		return nil, p.wrapError("CreateCreditNote", err)
	}

	return &Invoice{
		ID:           resp.InvoiceID,
		Number:       resp.InvoiceNo,
		CustomerName: input.CustomerName,
		CustomerID:   resp.CustomerID,
		DocDate:      input.DocDate,
		DueDate:      input.DueDate,
		Currency:     input.Currency,
		ReferenceNo:  resp.RefNo,
		Status:       InvoiceStatusUnpaid,
	}, nil
}

// --- Purchases ---

func (p *meritProvider) CreatePurchase(ctx context.Context, input CreatePurchaseInput) (*PurchaseInvoice, error) {
	rows, taxes := buildRowsAndTaxes(input.Lines)

	req := merit.CreatePurchaseRequest{
		Vendor: merit.VendorRef{
			ID:          input.VendorID,
			Name:        input.VendorName,
			RegNo:       input.VendorRegNo,
			Email:       input.VendorEmail,
			Address:     input.VendorAddress,
			CountryCode: input.VendorCountryCode,
		},
		DocDate:      formatDate(input.DocDate),
		DueDate:      formatDate(input.DueDate),
		BillNo:       input.BillNo,
		RefNo:        input.RefNo,
		CurrencyCode: input.Currency,
		InvoiceRow:   rows,
		TaxAmount:    taxes,
		Hcomment:     input.Comment,
		Fcomment:     input.FooterComment,
	}

	resp, err := p.client.CreatePurchase(ctx, req)
	if err != nil {
		return nil, p.wrapError("CreatePurchase", err)
	}

	return &PurchaseInvoice{
		ID:          resp.BillID,
		Number:      resp.BillNo,
		VendorName:  input.VendorName,
		VendorID:    resp.VendorID,
		DocDate:     input.DocDate,
		DueDate:     input.DueDate,
		Currency:    input.Currency,
		ReferenceNo: resp.RefNo,
		Status:      InvoiceStatusUnpaid,
	}, nil
}

func (p *meritProvider) GetPurchase(ctx context.Context, id string) (*PurchaseInvoice, error) {
	detail, err := p.client.GetPurchase(ctx, merit.GetInvoiceParams{ID: id})
	if err != nil {
		return nil, p.wrapError("GetPurchase", err)
	}
	return mapPurchaseDetail(detail), nil
}

func (p *meritProvider) ListPurchases(ctx context.Context, input ListPurchasesInput) ([]PurchaseInvoice, error) {
	items, err := p.client.ListPurchases(ctx, merit.ListPurchasesParams{
		PeriodStart: input.PeriodStart,
		PeriodEnd:   input.PeriodEnd,
	})
	if err != nil {
		return nil, p.wrapError("ListPurchases", err)
	}

	purchases := make([]PurchaseInvoice, len(items))
	for i, item := range items {
		purchases[i] = mapPurchaseListItem(item)
	}
	return purchases, nil
}

func (p *meritProvider) DeletePurchase(ctx context.Context, id string) error {
	err := p.client.DeletePurchase(ctx, merit.DeletePurchaseParams{ID: id})
	return p.wrapError("DeletePurchase", err)
}

// --- Reference data ---

func (p *meritProvider) ListTaxes(ctx context.Context) ([]Tax, error) {
	items, err := p.client.ListTaxes(ctx)
	if err != nil {
		return nil, p.wrapError("ListTaxes", err)
	}

	taxes := make([]Tax, len(items))
	for i, item := range items {
		taxes[i] = Tax{
			ID:   item.TaxID,
			Code: item.Code,
			Name: item.Name,
			Pct:  item.TaxPct,
		}
	}
	return taxes, nil
}

func (p *meritProvider) ListAccounts(ctx context.Context) ([]Account, error) {
	items, err := p.client.ListAccounts(ctx)
	if err != nil {
		return nil, p.wrapError("ListAccounts", err)
	}

	accounts := make([]Account, len(items))
	for i, item := range items {
		accounts[i] = Account{
			ID:     item.AccountID,
			Code:   item.Code,
			Name:   item.Name,
			Active: item.NonActive != "1",
		}
	}
	return accounts, nil
}

// --- Reports ---

func (p *meritProvider) CustomerDebts(ctx context.Context, customerName string, overdueDays *int) ([]CustomerDebt, error) {
	params := merit.CustomerDebtsParams{
		CustName:    customerName,
		OverDueDays: overdueDays,
	}

	items, err := p.client.CustomerDebts(ctx, params)
	if err != nil {
		return nil, p.wrapError("CustomerDebts", err)
	}

	debts := make([]CustomerDebt, len(items))
	for i, item := range items {
		debts[i] = CustomerDebt{
			CustomerName: item.PartnerName,
			CustomerID:   item.PartnerID,
			DocType:      item.DocType,
			DocDate:      parseDate(item.DocDate),
			DocNo:        item.DocNo,
			DueDate:      parseDate(item.DueDate),
			TotalAmount:  item.TotalAmount,
			PaidAmount:   item.PaidAmount,
			UnpaidAmount: item.UnPaidAmount,
			Currency:     item.CurrencyCode,
		}
	}
	return debts, nil
}

// --- Sync ---

func (p *meritProvider) ListInvoicesSince(ctx context.Context, since time.Time, until time.Time) ([]Invoice, error) {
	items, err := p.client.ListInvoices(ctx, merit.ListInvoicesParams{
		PeriodStart: since,
		PeriodEnd:   until,
		DateType:    intPtr(1), // 1 = changed date
	})
	if err != nil {
		return nil, p.wrapError("ListInvoicesSince", err)
	}

	invoices := make([]Invoice, len(items))
	for i, item := range items {
		invoices[i] = mapInvoiceListItem(item)
	}
	return invoices, nil
}

func (p *meritProvider) ListPaymentsSince(ctx context.Context, since time.Time, until time.Time) ([]Payment, error) {
	items, err := p.client.ListPayments(ctx, merit.ListPaymentsParams{
		PeriodStart: since,
		PeriodEnd:   until,
		DateType:    intPtr(1), // 1 = changed date
	})
	if err != nil {
		return nil, p.wrapError("ListPaymentsSince", err)
	}

	payments := make([]Payment, len(items))
	for i, item := range items {
		payments[i] = mapPaymentListItem(item)
	}
	return payments, nil
}

// --- Mapping helpers ---

func mapInvoiceListItem(item merit.InvoiceListItem) Invoice {
	return Invoice{
		ID:           item.SIHId,
		Number:       item.InvoiceNo,
		CustomerName: item.CustomerName,
		CustomerID:   item.CustomerID,
		DocDate:      parseDate(item.DocumentDate),
		DueDate:      parseDate(item.DueDate),
		TotalAmount:  item.TotalAmount,
		TaxAmount:    item.TaxAmount,
		PaidAmount:   item.PaidAmount,
		Currency:     item.CurrencyCode,
		Paid:         item.Paid,
		Status:       deriveInvoiceStatus(item.Paid, item.PaidAmount),
		ReferenceNo:  item.ReferenceNo,
	}
}

func mapInvoiceDetail(d *merit.InvoiceDetail) *Invoice {
	lines := make([]InvoiceLine, len(d.Lines))
	for i, row := range d.Lines {
		lines[i] = InvoiceLine{
			ID:            row.SILId,
			Description:   row.Description,
			Quantity:      row.Quantity,
			UnitPrice:     row.Price,
			TaxID:         row.TaxID,
			TaxName:       row.TaxName,
			TaxPct:        row.TaxPct,
			AmountExclVat: row.AmountExclVat,
			AmountInclVat: row.AmountInclVat,
			VatAmount:     row.VatAmount,
			AccountCode:   row.AccountCode,
		}
	}

	payments := make([]InvoicePayment, len(d.Payments))
	for i, pm := range d.Payments {
		payments[i] = InvoicePayment{
			Date:      parseDate(pm.PaymDate),
			Amount:    pm.Amount,
			Method:    pm.PaymentMethod,
			PaymentID: pm.PaymentID,
		}
	}

	return &Invoice{
		ID:           d.SIHId,
		Number:       d.InvoiceNo,
		CustomerName: d.CustomerName,
		CustomerID:   d.CustomerID,
		DocDate:      parseDate(d.DocumentDate),
		DueDate:      parseDate(d.DueDate),
		TotalAmount:  d.TotalAmount,
		TaxAmount:    d.TaxAmount,
		PaidAmount:   d.PaidAmount,
		Currency:     d.CurrencyCode,
		Paid:         d.Paid,
		Status:       deriveInvoiceStatus(d.Paid, d.PaidAmount),
		ReferenceNo:  d.ReferenceNo,
		Lines:        lines,
		Payments:     payments,
	}
}

func mapCustomerListItem(item merit.CustomerListItem) Customer {
	return Customer{
		ID:          item.CustomerID,
		Name:        item.Name,
		RegNo:       item.RegNo,
		VATRegNo:    item.VatRegNo,
		Email:       item.Email,
		Phone:       item.PhoneNo,
		Address:     item.Address,
		City:        item.City,
		County:      item.County,
		PostalCode:  item.PostalCode,
		CountryCode: item.CountryCode,
		Currency:    item.CurrencyCode,
		PaymentDays: item.PaymentDeadLine,
		Contact:     item.Contact,
		HomePage:    item.HomePage,
	}
}

func mapPaymentListItem(item merit.PaymentListItem) Payment {
	links := make([]PaymentInvoiceLink, len(item.PaymAPIDetails))
	for i, d := range item.PaymAPIDetails {
		links[i] = PaymentInvoiceLink{
			InvoiceID: d.DocID,
			InvoiceNo: d.DocNo,
			Amount:    d.PaidAmount,
		}
	}

	return Payment{
		ID:              item.PIHId,
		DocumentNo:      item.DocumentNo,
		DocumentDate:    parseDate(item.DocumentDate),
		Amount:          item.Amount,
		Currency:        item.CurrencyCode,
		Direction:       mapPaymentDirection(item.Direction),
		CounterPartID:   item.CounterPartID,
		CounterPartName: item.CounterPartName,
		InvoiceLinks:    links,
	}
}

func deriveInvoiceStatus(paid bool, paidAmount decimal.Decimal) InvoiceStatus {
	if paid {
		return InvoiceStatusPaid
	}
	if paidAmount.IsPositive() {
		return InvoiceStatusPartial
	}
	return InvoiceStatusUnpaid
}

func mapPaymentDirection(d int) PaymentDirection {
	switch d {
	case merit.DirectionCustomers:
		return PaymentDirectionCustomer
	case merit.DirectionVendors:
		return PaymentDirectionVendor
	case merit.DirectionOtherIncome:
		return PaymentDirectionOtherIncome
	case merit.DirectionOtherExpenses:
		return PaymentDirectionOtherExpense
	default:
		return PaymentDirectionCustomer
	}
}

func mapItemTypeToMerit(t ItemType) int {
	switch t {
	case ItemTypeStock:
		return merit.ItemTypeStock
	case ItemTypeService:
		return merit.ItemTypeService
	case ItemTypeItem:
		return merit.ItemTypeItem
	default:
		return merit.ItemTypeItem
	}
}

func mapMeritItemType(t int) ItemType {
	switch t {
	case merit.ItemTypeStock:
		return ItemTypeStock
	case merit.ItemTypeService:
		return ItemTypeService
	case merit.ItemTypeItem:
		return ItemTypeItem
	default:
		return ItemTypeItem
	}
}

func mapItemListItem(item merit.ItemListItem) Item {
	return Item{
		ID:            item.ItemID,
		Code:          item.Code,
		Name:          item.Name,
		Description:   item.Name,
		Type:          mapMeritItemType(item.Type),
		UnitOfMeasure: item.UnitOfMeasureName,
		SalesPrice:    item.SalesPrice,
	}
}

func mapPurchaseListItem(item merit.PurchaseListItem) PurchaseInvoice {
	return PurchaseInvoice{
		ID:          item.PIHId,
		Number:      item.BillNo,
		VendorName:  item.VendorName,
		VendorID:    item.VendorID,
		DocDate:     parseDate(item.DocumentDate),
		DueDate:     parseDate(item.DueDate),
		TotalAmount: item.TotalAmount,
		TaxAmount:   item.TaxAmount,
		PaidAmount:  item.PaidAmount,
		Currency:    item.CurrencyCode,
		Paid:        item.Paid,
		Status:      deriveInvoiceStatus(item.Paid, item.PaidAmount),
		ReferenceNo: item.ReferenceNo,
	}
}

func mapPurchaseDetail(d *merit.InvoiceDetail) *PurchaseInvoice {
	return &PurchaseInvoice{
		ID:          d.SIHId,
		Number:      d.InvoiceNo,
		VendorName:  d.CustomerName,
		VendorID:    d.CustomerID,
		DocDate:     parseDate(d.DocumentDate),
		DueDate:     parseDate(d.DueDate),
		TotalAmount: d.TotalAmount,
		TaxAmount:   d.TaxAmount,
		PaidAmount:  d.PaidAmount,
		Currency:    d.CurrencyCode,
		Paid:        d.Paid,
		Status:      deriveInvoiceStatus(d.Paid, d.PaidAmount),
		ReferenceNo: d.ReferenceNo,
	}
}

// wrapError converts Merit APIError status codes into sentinel errors.
func (p *meritProvider) wrapError(op string, err error) error {
	if err == nil {
		return nil
	}

	var apiErr *merit.APIError
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
		return &ProviderError{Provider: "merit", Op: op, Err: sentinel}
	}

	return &ProviderError{Provider: "merit", Op: op, Err: err}
}
