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

### Module Management
```bash
# Add a new dependency
go get package-name

# Update dependencies
go get -u ./...

# Tidy up go.mod/go.sum
go mod tidy
```

## Architecture

### Provider Pattern
The library uses a **Provider interface** (`provider.go`) that defines all accounting operations. Each accounting backend implements this interface, enabling multi-provider support without changing the public API.

- **Provider interface**: Defines methods for all operations (invoices, customers, payments, items, purchases, taxes, reports, sync)
- **meritProvider** (`merit_adapter.go`): Implements Provider using Merit Aktiva API
- Future providers will implement the same interface

### Client and Services Pattern
The main entry point is `Client` (`client.go`), created via `NewClient(Config)`:
- Takes a `Config` specifying provider, credentials, region, and optional HTTP client
- Returns a `Client` with service fields: `Invoices`, `Customers`, `Payments`, `Items`, `Purchases`, `Taxes`, `Reports`, `Sync`
- Each service (e.g., `InvoiceService` in `invoice_service.go`) wraps the provider and provides domain-specific methods
- Services delegate to the provider implementation

### Adapter Pattern
Each provider has an adapter (e.g., `merit_adapter.go`) that:
- Translates library types (defined in `types.go`) to/from provider-specific types
- Maps provider-specific errors to library sentinel errors (`ErrNotFound`, `ErrAuthFailed`, `ErrRateLimit`)
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
Some services support batch operations (e.g., `InvoiceService.BatchCreate`) that process multiple items concurrently with a concurrency limit (semaphore pattern).

### Error Wrapping
Provider errors are wrapped in `ProviderError` (defined in `errors.go`) which includes:
- Provider name
- Operation name
- Underlying error (may be a sentinel error)

## Testing Conventions

Tests use `httptest` to mock HTTP responses:
```go
func newTestServer(t *testing.T, handler http.HandlerFunc) (*Client, *httptest.Server)
```

Test files verify:
- Authentication headers (ApiId, timestamp, signature)
- Request paths and methods
- Request/response payload structure
- Error handling and status codes

## Dependencies

- `github.com/shopspring/decimal`: Used for precise monetary calculations (amounts, percentages, taxes)

## Adding a New Provider

To add a new accounting provider:
1. Create a new package directory (e.g., `simplbooks/`)
2. Implement the `Provider` interface from `provider.go`
3. Create an adapter file in the root (e.g., `simplbooks_adapter.go`)
4. Map between library types (`types.go`) and provider-specific types
5. Add provider case to the switch statement in `client.go:NewClient()`
6. Implement error mapping to sentinel errors
