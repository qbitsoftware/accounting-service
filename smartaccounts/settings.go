package smartaccounts

import "context"

// ListVatPcs returns all VAT-percentage register entries (settings/vatpcs).
func (c *Client) ListVatPcs(ctx context.Context) ([]VatPc, error) {
	var items []VatPc
	if err := c.getAll(ctx, "settings/vatpcs:get", nil, &items); err != nil {
		return nil, err
	}
	return items, nil
}

// ListAccounts returns all general-ledger accounts (settings/accounts).
func (c *Client) ListAccounts(ctx context.Context) ([]AccountItem, error) {
	var items []AccountItem
	if err := c.getAll(ctx, "settings/accounts:get", nil, &items); err != nil {
		return nil, err
	}
	return items, nil
}

// ListBankAccounts returns all bank accounts (settings/bankaccounts).
func (c *Client) ListBankAccounts(ctx context.Context) ([]BankAccountItem, error) {
	var items []BankAccountItem
	if err := c.getAll(ctx, "settings/bankaccounts:get", nil, &items); err != nil {
		return nil, err
	}
	return items, nil
}

// ListObjects returns all accounting objects/dimensions (settings/objects).
func (c *Client) ListObjects(ctx context.Context) ([]ObjectItem, error) {
	var items []ObjectItem
	if err := c.getAll(ctx, "settings/objects:get", nil, &items); err != nil {
		return nil, err
	}
	return items, nil
}
