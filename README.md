# hris-kit

Shared Go library for the HRIS Management System. It holds the small, cross-cutting
pieces that every service in the system needs — structured logging, environment
config, error codes, paging rules, and formatting helpers — so they are defined
once instead of being re-implemented (and drifting) in each service.

```
module github.com/rvldodo/hris-kit
```

This is a library module. It has no `main`, no HTTP server, and no database
driver: it stays framework-agnostic so `api`, `api-web`, and any future service
can depend on it without inheriting each other's stack.

## Packages

| Package | What it's for |
| --- | --- |
| [`logger`](logger/logger.go) | zap-based structured logging, plus request-scoped context (request ID, user ID, caller name) |
| [`env`](env/config.go) | Typed environment variable reads with fallbacks; auto-loads `.env` |
| [`errors`](errors/error.go) | `AppError` — an error code registry mapping each failure to an HTTP status and an internal status code |
| [`lib`](lib/lib.go) | Nil-safe conversions, parsing, invoice-number builders, paging normalization, currency formatting |
| [`constant`](constant/const.go) | Shared constants (paging defaults and limits) |

## Install

The module path is `github.com/rvldodo/hris-kit`, while the repository lives at
`github.com/hris-management-system/go-kit`. Until those line up, consuming
services need a `replace` directive:

```go
// go.mod in services/api
require github.com/rvldodo/hris-kit v0.0.0

replace github.com/rvldodo/hris-kit => ../go-kit
```

Then:

```bash
go mod tidy
```

Requires Go 1.26.1 or newer.

## Usage

### logger

Initialize once at service startup, then log through the context so every line
carries the request's identifiers.

```go
package main

import (
    "context"
    "github.com/rvldodo/hris-kit/logger"
)

func main() {
    if err := logger.Init("production"); err != nil { // "production" or anything else for dev
        panic(err)
    }
    defer logger.Sync()

    logger.Log.Info("service started")
}
```

`Init("production")` gives JSON output with ISO8601 timestamps. Any other value
gives human-readable, colored development output.

Attach identifiers to the context — typically in middleware — and `WithContext`
picks them up automatically:

```go
func Middleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        ctx := logger.SetRequestIDCtx(r.Context())
        ctx = logger.SetDeviceCtx(ctx, r.Header.Get("X-Device"))
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}

func (s *Service) GetEmployee(ctx context.Context, id int64) error {
    ctx = logger.SetFuncNameCtx(ctx) // records "GetEmployee"
    logger.WithContext(ctx).Info("fetching employee", zap.Int64("id", id))
    // → {"msg":"fetching employee","requestID":"a1b2c3d4","funcName":"GetEmployee","id":42}
}
```

Available context fields: `requestID`, `userID`, `partnerID`, `funcName`,
`device`, `appVersion`. Each has a `Set…Ctx` / `Get…Ctx` pair.
`SetFuncNameCtx` records the caller of the function that calls it; use
`SetFuncNameHandlerCtx` from one frame deeper (an HTTP handler wrapper).

Use `SugarWithContext(ctx)` if you prefer the printf-style sugared logger.

### env

`.env` is loaded automatically on import. A missing `.env` is fine — env vars
injected directly by the platform work the same way. A `.env` that exists but
cannot be read panics, because that is a misconfiguration rather than a choice.

```go
port    := env.GetString("PORT", "8080")
maxConn := env.GetInt("DB_MAX_CONN", 10)
timeout := env.GetInt64("REQUEST_TIMEOUT_MS", 5000)
debug   := env.GetBool("DEBUG", false)          // true/t/yes/y/1/on
origins := env.GetStringArray("CORS_ORIGINS", []string{"*"}) // comma-separated
```

Every getter returns the fallback when the variable is unset *or* unparseable —
reads never fail, so callers don't need error handling for config.

### errors

Failures are represented as a `Code` rather than an ad-hoc string, so the HTTP
status and the internal status code are decided in one place.

```go
// in a repository
row, err := db.QueryRow(...)
if err == sql.ErrNoRows {
    return errors.New(errors.SQLDataIsNotFound)
}
if err != nil {
    return errors.Wrap(errors.FailToSelect, err) // keeps the cause for logs
}
```

At the HTTP boundary, translate it into a response:

```go
var appErr *errors.AppError
if goerrors.As(err, &appErr) {
    d := errors.Detail(appErr.Code)
    w.WriteHeader(d.HttpStatus)
    json.NewEncoder(w).Encode(map[string]any{
        "status":  d.InternalStatus,
        "message": d.Message,
    })
    logger.WithContext(ctx).Error("request failed", zap.Error(appErr)) // cause included
}
```

`AppError` implements `Unwrap`, so `errors.Is` / `errors.As` reach the wrapped
cause. An unregistered code falls back to `Undefined` (HTTP 500) instead of
panicking.

Codes cover request validation (4001–4005), auth (4010–4017), not-found and
conflict (4040–4091), forbidden keys (4030–4031), rate limiting (4290), device
claiming, and internal failures (5000–5012). Add a new one by appending to the
`Code` const block and registering its `CodeDetail` in the `codes` map — append
only, since the values are `iota`-based.

Note that several auth and claim codes are deliberately coarse: unknown,
expired, and already-consumed claim codes all return `InvalidClaimCode` so that
probing cannot confirm whether a guessed code ever existed. Don't split these
for the sake of nicer error messages.

### lib

Conversions and parsers that return a usable zero value instead of an error,
for the many call sites where a bad value should not abort the request:

```go
lib.DerefString(user.Email)      // "" when the column is NULL
lib.GetStringPointer("")         // nil — for writing a nullable column
lib.ParseFloat("1,250.50")       // 1250.5 (strips thousands separators)
lib.ParseInt("42")               // 42, or 0 if unparseable
lib.ParseFloatPointer("")        // nil
lib.IsTruthy("yes")              // true
lib.IsTruthyBit("yes")           // 1 — for BIT columns
```

Paging, clamped to the limits in `constant`:

```go
limit, offset := lib.NormalizePaging(req.Page, req.PageSize)
// page < 1        → page 1
// pageSize < 1    → 20 (an absent query param means "default", not "none")
// pageSize > 100  → 100
```

Formatting and identifiers:

```go
lib.ConvertToRp(1500000)    // "Rp1.500.000"
lib.ConvertToMl(250)        // "250 ml"
lib.ConvertToPoint(1200)    // "1.200 poin"
lib.Separate3Digits(1000)   // "1.000" (Indonesian separator)

lib.GenerateVerificationCode()                  // 4-digit code, e.g. "4821"
lib.GenerateOrderInvoice("20260922", 1042)      // "WH-ODR-INV/20260922/1042"
lib.GeneratePaymentInvoice("20260922", 1042)    // "WH-PYM-INV/20260922/1042"
lib.GenerateOrderInvoiceDoor("20260922", 1042)  // "WH-ODR-DS-INV/..." (door-to-door)
```

The `Door` variants use a distinct prefix so an invoice number identifies the
issuing system without a database lookup.

`ParseDateToDateTime2` parses `MM/DD/YYYY` or `MM/DD/YY` and stamps it with the
current wall-clock time, mirroring SQL Server's `SYSDATETIME()` behavior:

```go
t, err := lib.ParseDateToDateTime2("01/15/2026")
```

### constant

```go
constant.DEFAULT_PAGE      // 1
constant.DEFAULT_PAGE_SIZE // 20
constant.MAX_PAGE_SIZE     // 100
```

## Development

```bash
make tidy   # go mod tidy
go build ./...
go test ./...
```

## Conventions

- **Library only.** No `main`, no server, no DB driver. If something needs a
  database connection or an HTTP router, it belongs in a service, not here.
- **Additive error codes.** `Code` is `iota`-based, so inserting a code in the
  middle renumbers every code after it. Always append.
- **Helpers degrade, they don't fail.** `env` and `lib` return fallbacks and
  zero values. Reserve `error` for the genuinely exceptional.
- **Log through the context.** `logger.WithContext(ctx)` rather than
  `logger.Log` directly, so request correlation survives across layers.
