package accounting

// Capabilities describes which optional features a provider supports.
// Consumers (UIs, sync services) read this to hide actions the active
// provider cannot perform — e.g. PDF download or invoice deletion for
// Excellent Books, which the API does not expose.
type Capabilities struct {
	SupportsInvoicePDF       bool `json:"supports_invoice_pdf"`
	SupportsInvoiceDelete    bool `json:"supports_invoice_delete"`
	SupportsPaymentDelete    bool `json:"supports_payment_delete"`
	SupportsPurchaseCreate   bool `json:"supports_purchase_create"`
	SupportsPurchaseDelete   bool `json:"supports_purchase_delete"`
	SupportsTaxList          bool `json:"supports_tax_list"`
	SupportsAccountList      bool `json:"supports_account_list"`
	SupportsDimensions       bool `json:"supports_dimensions"`
	SupportsCustomerDebts    bool `json:"supports_customer_debts"`
	SupportsVendorPayments   bool `json:"supports_vendor_payments"`
	SupportsFindInvoiceByRef bool `json:"supports_find_invoice_by_ref"`
	SupportsIncrementalSync  bool `json:"supports_incremental_sync"` // True if ListInvoicesSince / ListPaymentsSince track changes (not just doc-date range)
}

// ProviderCapabilities returns the capability set for a given provider name.
// Unknown providers return zero-value Capabilities (all features disabled).
//
// This is exposed as a free function so consumers can query capabilities
// without instantiating a Client (e.g. when rendering a provider-selection UI).
func ProviderCapabilities(provider string) Capabilities {
	switch provider {
	case "merit":
		return Capabilities{
			SupportsInvoicePDF:       true,
			SupportsInvoiceDelete:    true,
			SupportsPaymentDelete:    true,
			SupportsPurchaseCreate:   true,
			SupportsPurchaseDelete:   true,
			SupportsTaxList:          true,
			SupportsAccountList:      true,
			SupportsDimensions:       true,
			SupportsCustomerDebts:    true,
			SupportsVendorPayments:   true,
			SupportsFindInvoiceByRef: false,
			SupportsIncrementalSync:  true,
		}
	case "excellentbooks":
		return Capabilities{
			SupportsInvoicePDF:       false,
			SupportsInvoiceDelete:    false,
			SupportsPaymentDelete:    false,
			SupportsPurchaseCreate:   false,
			SupportsPurchaseDelete:   false,
			SupportsTaxList:          true,
			SupportsAccountList:      true,
			SupportsDimensions:       true,
			SupportsCustomerDebts:    false,
			SupportsVendorPayments:   false,
			SupportsFindInvoiceByRef: true,
			SupportsIncrementalSync:  false,
		}
	case "directo":
		return Capabilities{
			SupportsInvoicePDF:       false,
			SupportsInvoiceDelete:    true,
			SupportsPaymentDelete:    true,
			SupportsPurchaseCreate:   true,
			SupportsPurchaseDelete:   true,
			SupportsTaxList:          true,
			SupportsAccountList:      true,
			SupportsDimensions:       true,
			SupportsCustomerDebts:    true,
			SupportsVendorPayments:   true,
			SupportsFindInvoiceByRef: true,
			SupportsIncrementalSync:  false,
		}
	}
	return Capabilities{}
}
