package smartaccounts

import (
	"context"
	"net/url"

	"github.com/shopspring/decimal"
)

// ListClientsParams filters a clients:get query.
type ListClientsParams struct {
	NameOrRegCode string
	ID            string
	ModifiedFrom  string // dd.MM.yyyy
	ModifiedTo    string // dd.MM.yyyy
	FetchContacts bool
	FetchAddress  bool
}

func (p ListClientsParams) values() url.Values {
	v := url.Values{}
	setNonEmpty(v, "nameOrRegCode", p.NameOrRegCode)
	setNonEmpty(v, "id", p.ID)
	setNonEmpty(v, "modifiedFrom", p.ModifiedFrom)
	setNonEmpty(v, "modifiedTo", p.ModifiedTo)
	if p.FetchContacts {
		v.Set("fetchContacts", "true")
	}
	if p.FetchAddress {
		v.Set("fetchAddresses", "true")
	}
	return v
}

// ListClients retrieves clients matching params, following pagination.
func (c *Client) ListClients(ctx context.Context, params ListClientsParams) ([]ClientItem, error) {
	// Always fetch contacts/addresses so email/phone/address are populated.
	params.FetchContacts = true
	params.FetchAddress = true
	var items []ClientItem
	if _, err := c.getList(ctx, "purchasesales/clients:get", params.values(), &items); err != nil {
		return nil, err
	}
	return items, nil
}

// GetClient fetches a single client by ID.
func (c *Client) GetClient(ctx context.Context, id string) (*ClientItem, error) {
	items, err := c.ListClients(ctx, ListClientsParams{ID: id})
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, &APIError{StatusCode: 404, Message: "client not found: " + id}
	}
	return &items[0], nil
}

// CreateClient adds a client.
func (c *Client) CreateClient(ctx context.Context, req CreateClientRequest) (*ClientResponse, error) {
	var resp ClientResponse
	if err := c.post(ctx, "purchasesales/clients:add", nil, req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// EditClient updates a client (req.ID must be set).
func (c *Client) EditClient(ctx context.Context, req CreateClientRequest) error {
	return c.post(ctx, "purchasesales/clients:edit", nil, req, nil)
}

// ClientBalance is the response of clients:getbalance (with fetchDetails=true).
type ClientBalance struct {
	ID                string             `json:"id"`
	BalanceDate       string             `json:"balanceDate"`
	PrepaymentsUnused []PrepaymentUnused `json:"prepaymentsUnusedList"`
	InvoicesUnpaid    []InvoiceUnpaid    `json:"invoicesUnpaidList"`
}

// PrepaymentUnused is an unallocated customer advance from the balance report.
// UnusedAmount is the still-unapplied remainder and is NEGATIVE (a liability),
// so callers take its magnitude. PaymentID is the parked payment's id — the
// stable handle referenced as a PREPAYMENT_PAYMENT row id when applying it.
type PrepaymentUnused struct {
	PaymentID    string          `json:"paymentId"`
	UnusedAmount decimal.Decimal `json:"unusedAmount"`
	Currency     string          `json:"currency"`
	Description  string          `json:"description"`
	PaymentDate  string          `json:"paymentDate"`
}

// InvoiceUnpaid is an open client invoice from the balance report.
type InvoiceUnpaid struct {
	ID                string          `json:"id"`
	InvoiceNumber     flexString      `json:"invoiceNumber"`
	Date              string          `json:"date"`
	DueDate           string          `json:"dueDate"`
	Currency          string          `json:"currency"`
	TotalAmount       decimal.Decimal `json:"totalAmount"`
	OutstandingAmount decimal.Decimal `json:"outstandingAmount"`
}

// GetClientBalance fetches a client's balance as of balanceDate. With
// details=true it includes the client's unallocated prepayments (advances) and
// unpaid invoices — the source for ListPrepayments on SmartAccounts, which has
// no standalone prepayment register.
func (c *Client) GetClientBalance(ctx context.Context, clientID, balanceDate string, details bool) (*ClientBalance, error) {
	v := url.Values{"id": {clientID}}
	setNonEmpty(v, "balanceDate", balanceDate)
	if details {
		v.Set("fetchDetails", "true")
	}
	var bal ClientBalance
	if err := c.get(ctx, "purchasesales/clients:getbalance", v, &bal); err != nil {
		return nil, err
	}
	return &bal, nil
}
