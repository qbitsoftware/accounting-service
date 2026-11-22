package accounting

import (
	"encoding/json"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/qbitsoftware/accounting-service/merit"
)

// Merit's getitems as the test company answered it (22.11.2026) for a stock
// item after a receipt of 10 and a sale of 2.
func TestMeritItemCarriesStock(t *testing.T) {
	var raw merit.ItemListItem
	if err := json.Unmarshal([]byte(`{"ItemId":"i1","Code":"KLB-SKU-1023","Name":"Probe jersey M","Type0":1,"Type":"Laokaup",
		"InventoryQty":8.0,"ReservedQty":1.0,"ItemUnitCost":8.0,"SalesPrice":0.0}`), &raw); err != nil {
		t.Fatal(err)
	}
	it := mapItemListItem(raw)
	if it.Type != ItemTypeStock || !it.InventoryQty.Equal(decimal.NewFromInt(8)) || !it.ReservedQty.Equal(decimal.NewFromInt(1)) ||
		!it.UnitCost.Equal(decimal.NewFromInt(8)) {
		t.Errorf("item = %+v", it)
	}
}

// A line's per-unit cost reaches the row; a line without one keeps the zero
// every row has always carried.
func TestMeritRowCarriesItemCost(t *testing.T) {
	one := 1
	rows, _ := buildRowsAndTaxes([]CreateInvoiceLineInput{
		{Code: "KLB-SKU-1023", Quantity: decimal.NewFromInt(-1), UnitPrice: decimal.NewFromInt(25), Type: &one, ItemCostAmount: decimal.NewFromInt(8)},
		{Code: "TRENN1", Quantity: decimal.NewFromInt(1), UnitPrice: decimal.NewFromInt(40)},
	})
	if !rows[0].ItemCostAmount.Equal(decimal.NewFromInt(8)) || !rows[1].ItemCostAmount.IsZero() {
		t.Errorf("item costs = %s, %s", rows[0].ItemCostAmount, rows[1].ItemCostAmount)
	}
}
