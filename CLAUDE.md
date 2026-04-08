# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a Go library (`github.com/qbitsoftware/accounting-service`) that provides a unified interface for accounting and bookkeeping operations across multiple accounting service providers. Currently supports Merit Aktiva (Estonia/Poland) with SimplBooks planned for future implementation.

## Common Commands

### Building
```bash
go build ./...
```

### Testing
```bash
# Run all tests
go test ./...

# Run tests in a specific package
go test ./merit

# Run tests with verbose output
go test -v ./...

# Run a single test
go test -run TestName ./merit
```

## Architecture

### Provider Pattern
The library uses a **Provider interface** (`provider.go`) that defines all accounting operations. Each accounting backend implements this interface, enabling multi-provider support without changing the public API.

- **Provider interface**: Defines methods for all operations (invoices, customers, payments, items, purchases, taxes, reports, sync)
- **meritProvider** (`merit_adapter.go`): Implements Provider using Merit Aktiva API
- `simplbooks/` directory exists as a placeholder for future SimplBooks provider

### Client and Services Pattern
The main entry point is `Client` (`client.go`), created via `NewClient(Config)`:
- Takes a `Config` specifying provider, credentials, region, and optional HTTP client
- Returns a `Client` with service fields: `Invoices`, `Customers`, `Payments`, `Items`, `Purchases`, `Taxes`, `Reports`, `Sync`
- Each service (e.g., `InvoiceService` in `invoice_service.go`) wraps the provider and provides domain-specific methods
- Services delegate to the provider implementation

### Key Root-Level Files
- `types.go`: Library-level domain types (Invoice, Customer, Payment, Item, etc.)
- `inputs.go`: All input structs for create/update/list operations
- `errors.go`: Sentinel errors and `ProviderError` wrapper
- `reference.go`: Estonian reference number (viitenumber) generation using 3-7-1 algorithm
- `helpers.go`: Date parsing/formatting utilities (YYYYMMDD format constant: `meritDateFormat`)

### Adapter Pattern
Each provider has an adapter (e.g., `merit_adapter.go`) that:
- Translates library types to/from provider-specific types (e.g., `merit/types.go`)
- Maps HTTP status codes to sentinel errors (401/403→`ErrAuthFailed`, 404→`ErrNotFound`, 429→`ErrRateLimit`)
- Handles provider-specific authentication and request formatting

### Merit Provider Implementation
Located in `merit/` directory:
- `merit/client.go`: Core Merit API client with regional endpoint support (Estonia, Poland)
- `merit/auth.go`: HMAC-SHA256 authentication implementation
- `merit/request.go`: HTTP request building and error handling
- Domain-specific files: `invoices.go`, `customers.go`, `payments.go`, `items.go`, `purchases.go`, `vendors.go`, `taxes.go`, `accounts.go`, `reports.go`
- `merit/types.go`: Merit API request/response types
- `merit/merit_test.go`: Comprehensive test suite using httptest for mocking

## Key Design Patterns

### Regional Endpoint Support
Merit Aktiva operates in multiple regions with different base URLs:
- Estonia: `https://aktiva.merit.ee/api/`
- Poland: `https://program.360ksiegowosc.pl/api/`

The client configuration supports region selection via the `Region` field in `Config`.

### Batch Operations
`InvoiceService.BatchCreate` processes multiple invoices concurrently using a channel-based semaphore (limit: 5 concurrent requests).

### FindOrCreate Pattern
`CustomerService.FindOrCreate` searches by email first, creates if `IsNotFound()` returns true. This pattern avoids duplicate customers.

### Invoice Line Dimensions
Two approaches for attaching dimensions (projects, cost centers) to invoice lines:
- **v1 flat fields**: `ProjectCode` and `CostCenterCode` on `CreateInvoiceLineInput`
- **v2 Dimensions array**: `[]LineDimension` with `DimID`, `DimValueID`, `DimCode`

### Error Wrapping
Provider errors are wrapped in `ProviderError` (defined in `errors.go`) which includes:
- Provider name
- Operation name
- Underlying error (may be a sentinel error)

### Merit Authentication
HMAC-SHA256 scheme: signs `apiID + timestamp(YYYYMMDDHHmmss) + jsonBody` with the API key. Signature, ApiId, and timestamp are passed as URL query parameters.

## Testing Conventions

Tests use `httptest` to mock HTTP responses:
```go
func newTestServer(t *testing.T, handler http.HandlerFunc) (*Client, *httptest.Server)
```

Test files:
- `merit/merit_test.go`: Auth headers, request paths, payload structure, error handling
- `reference_test.go`: Estonian reference number validation
- `example_test.go`: End-to-end workflow example (customer → item → invoice creation)

## Adding a New Provider

To add a new accounting provider:
1. Create a new package directory (e.g., `simplbooks/`)
2. Implement the `Provider` interface from `provider.go`
3. Create an adapter file in the root (e.g., `simplbooks_adapter.go`)
4. Map between library types (`types.go`) and provider-specific types
5. Add provider case to the switch statement in `client.go:NewClient()`
6. Implement error mapping to sentinel errors
