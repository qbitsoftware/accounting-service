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
- `merit/window.go`: period splitting for Merit's 3-month-per-call cap (`SplitPeriod`, `MaxPeriodDays`)
- `merit/range.go`: generic range walk + `PartialRangeError`
- `merit/limiter.go`: process-wide rate limiter shared per API ID
- `merit/merit_test.go`, `merit/hardening_test.go`: test suite using httptest for mocking

### Merit period limit — never build a window by hand

Merit rejects any list call whose period spans more than 3 months with HTTP 400
`Periood liiga pikk(max 3 kuud)`. **That is a limit on one call, not on how far
back Merit will look** — history of any age is reachable by asking for the
window it falls in.

- Use `ListInvoicesRange` / `ListPaymentsRange` / `ListPurchasesRange` (and the
  adapter methods that call them) for anything wider than a few weeks. They
  split the range via `SplitPeriod` into `MaxPeriodDays` (85) slices that tile
  it exactly — no gap that would drop a day's invoices, no overlap that would
  book a payment twice — and merge the results, deduplicated by id.
- The bare `ListInvoices` / `ListPayments` / `ListPurchases` are the raw
  single-call form and will 400 on a wide period. `MaxPeriodDays` is 85, not 90,
  because three *calendar* months can be as short as 89 days (1 Dec → 1 Mar).
- **A range walk returns partial results with its error.** When some windows
  succeed and others fail you get the fetched rows *and* a `*PartialRangeError`;
  when all fail you get no rows. Callers keeping the usual `if err != nil`
  shape are unaffected. Use `accounting.IsPartialResult(err)` to tell "nothing
  found" from "the window that held it failed" — those mean opposite things
  when reconciling payments. `merit.IsPeriodTooLong(err)` identifies the 400
  itself, for a caller that built its own window.

### Merit rate limiting and retries

Merit allows ~100 requests/minute **per API ID**, and callers here construct a
fresh `Client` per operation — so the limiter is shared process-wide, keyed by
API ID + host (`merit/limiter.go`), exactly as SmartAccounts' is. A limiter
living on one Client would throttle nothing the moment a scheduled pull, a
manual sync and a batch push overlap on the same credentials. The most
conservative configuration seen for a key wins. Still per-process: with several
replicas the quota splits again.

`merit/request.go` also retries 429/502/503/504 (honouring `Retry-After`,
otherwise 2s/8s/20s/45s), re-signing each attempt because the signature embeds
a timestamp. Other non-2xx statuses are answers about the request itself and are
never retried. `DisableRateLimit: true` turns throttling off for tests.

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
