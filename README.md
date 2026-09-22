# go-kit

Shared Go library for the HRIS Management System. It holds the small, cross-cutting
pieces that every service in the system needs — structured logging, environment
config, error codes, paging rules, and conversion helpers — so they are defined
once instead of being re-implemented (and drifting) in each service.

```
module github.com/hris-management-system/go-kit
```

This is a library module — no `main`, no server startup, no database driver.
The core packages (`logger`, `env`, `errors`, `lib`, `constant`) are plain Go
and carry no web framework. The HTTP-facing packages (`middlewares`,
`response`) are **gin-specific**, so depending on this kit commits a service to
gin for its transport layer.

## Packages

| Package | What it's for |
| --- | --- |
| [`logger`](logger/logger.go) | zap-based structured logging, plus request-scoped context (request ID, user ID, caller name) |
| [`env`](env/config.go) | Typed environment variable reads with fallbacks; auto-loads `.env` |
| [`errors`](errors/error.go) | `AppError` — an error code registry mapping each failure to an HTTP status and an internal status code |
| [`lib`](lib/lib.go) | Nil-safe conversions, lenient parsing, date handling, paging normalization |
| [`constant`](constant/const.go) | Shared constants (paging defaults and limits) |
| [`middlewares`](middlewares/) | gin middleware — request logging and rate limiting |
| [`response`](response/response.go) | gin response helpers; RFC 7807 problem details for errors |

## Install

Requires Go 1.26.1 or newer.

The repository is **private**, so `go get` needs to authenticate and must skip
the public proxy and checksum database. Configure this once per machine:

```bash
go env -w GOPRIVATE=github.com/hris-management-system/*

# fetch over SSH instead of HTTPS
git config --global url."git@github.com:".insteadOf "https://github.com/"
```

Then, from a consuming service:

```bash
go get github.com/hris-management-system/go-kit
```

Without `GOPRIVATE`, `go get` asks proxy.golang.org for a private module, gets
nothing, falls back to direct HTTPS, and fails with
`could not read Username for 'https://github.com'` — an auth error that reads
like a wrong import path.

### Local development

While working across `go-kit` and a service at the same time, point the service
at your working copy so you don't have to tag and push for every change:

```go
// go.mod in services/api
require github.com/hris-management-system/go-kit v0.1.0

replace github.com/hris-management-system/go-kit => ../go-kit
```

A `replace` reads straight from disk — no network, no auth, no tag. Drop it
before merging so CI builds the published version.

## Usage

### logger

Initialize once at service startup, then log through the context so every line
carries the request's identifiers.

```go
package main

import (
    "context"
    "github.com/hris-management-system/go-kit/logger"
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

`GetStringPointerStatus` is the reporting-facing counterpart to
`GetStringPointer`: instead of `nil`, an empty or placeholder value becomes a
readable marker, so an export shows why a cell is blank.

```go
lib.GetStringPointerStatus("")       // → "Unclean Data (No Call)"
lib.GetStringPointerStatus("active") // → "active"
```

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

### middlewares

gin middleware. Both are constructors — call them to get a `gin.HandlerFunc`.

```go
r := gin.New()
r.Use(middlewares.GinLogging())
```

`GinLogging` logs one line per request after it completes: status, latency,
client IP, method, and path.

#### Rate limiting

A token bucket per key, held in process memory. `RateLimit` returns the
middleware and the limiter behind it; keep the limiter so you can `Stop` its
eviction sweeper at shutdown.

```go
limiter, rl := middlewares.RateLimit(middlewares.RateLimitConfig{
    Requests: 60,
    Window:   time.Minute,
})
defer rl.Stop()

r.Use(limiter)
```

The zero config is usable — 100 requests per minute, keyed by client IP:

```go
limiter, rl := middlewares.RateLimit(middlewares.RateLimitConfig{})
```

| Field | Default | Meaning |
| --- | --- | --- |
| `Requests` | 100 | Allowed per `Window` |
| `Window` | 1 minute | Period `Requests` is counted over |
| `Burst` | `Requests` | How many may arrive back-to-back before the steady rate applies |
| `KeyFunc` | client IP | What is being limited |
| `Skip` | nil | Return true to bypass the limit for a request |
| `IdleTTL` | `10×Window`, min 1 minute | How long an inactive key is kept |
| `SweepInterval` | `IdleTTL` | How often eviction runs |

Because it is a token bucket rather than a fixed window, a client that has been
idle accumulates up to `Burst` requests it can spend at once, then settles to
`Requests` per `Window`. Idle time never banks more than `Burst`.

**Choosing a key.** The default limits per IP, which is the right default for
unauthenticated endpoints. For authenticated ones, limit per principal so that
users behind one office NAT don't share an allowance:

```go
middlewares.KeyByIP()                 // client IP (default)
middlewares.KeyByHeader("X-API-Key")  // per API key or device token
middlewares.KeyByContext("userID")    // per value your auth middleware stored
```

`KeyByHeader` and `KeyByContext` fall back to the client IP when the value is
absent, so a caller cannot escape the limit by omitting it. A custom `KeyFunc`
returning `""` is folded into one shared bucket for the same reason.

Different limits for different routes are separate limiters:

```go
// strict on login, relaxed elsewhere
loginLimit, loginRL := middlewares.RateLimit(middlewares.RateLimitConfig{
    Requests: 5, Window: time.Minute,
})
defer loginRL.Stop()
r.POST("/auth/login", loginLimit, handler.Login)

// skip health checks
apiLimit, apiRL := middlewares.RateLimit(middlewares.RateLimitConfig{
    Requests: 600, Window: time.Minute,
    KeyFunc: middlewares.KeyByContext("userID"),
    Skip: func(c *gin.Context) bool { return c.Request.URL.Path == "/health" },
})
defer apiRL.Stop()
r.Use(apiLimit)
```

A rejected request gets `429` with the standard problem body (from
`errors.RateLimited`, internal status `4290`) plus `Retry-After`,
`X-RateLimit-Limit`, `X-RateLimit-Remaining`, and `X-RateLimit-Reset`.

Two limits to know before relying on this:

- **Per process, not per cluster.** Behind N replicas a client gets N times the
  configured allowance. That is fine for shielding one instance from overload;
  it is not a correctness boundary. A shared quota needs a Redis-backed
  counter instead.
- **Only as trustworthy as the key.** IP keying uses `c.ClientIP()`, which
  honours `X-Forwarded-For` only for proxies registered via
  `(*gin.Engine).SetTrustedProxies`. Leave that unset behind a load balancer
  and every request looks like it came from the balancer, collapsing all
  clients into a single bucket. Set it, or key on something authenticated.

### response

Response helpers that keep every endpoint returning the same shapes. Success is
the bare payload; errors are [RFC 7807](https://datatracker.ietf.org/doc/html/rfc7807)
problem details.

```go
response.OK(c, employee)                       // 200, payload as-is
response.Created(c, employee)                  // 201
response.NoContent(c)                          // 204
response.Paginated(c, items, total, page, size) // 200 with data/total/page/page_size
```

`response.Err` is the one to reach for in handlers: it unwraps an `*AppError`
and derives the status and message from the registered code, falling back to
500 for anything else.

```go
if err != nil {
    response.Err(c, err) // 404 for SQLDataIsNotFound, 429 for RateLimited, ...
    return
}
```

Direct helpers exist when there is no error value to pass:
`BadRequest`, `NotFound`, `Unauthorized`, `Forbidden`, `Internal`, and
`ValidationFailed(c, []response.FieldError{...})` for field-level errors.

## Development

```bash
make tidy            # go mod tidy
go build ./...
go test ./... -race  # the rate limiter is concurrent; run it under -race
```

## Conventions

- **Library only.** No `main`, no server startup, no DB driver. If something
  needs a database connection or a live route table, it belongs in a service.
- **Keep the core framework-free.** `logger`, `env`, `errors`, `lib` and
  `constant` must not import gin. Anything gin-shaped goes in `middlewares` or
  `response`, so a non-gin service can still use the core.
- **Additive error codes.** `Code` is `iota`-based, so inserting a code in the
  middle renumbers every code after it. Always append.
- **Helpers degrade, they don't fail.** `env` and `lib` return fallbacks and
  zero values. Reserve `error` for the genuinely exceptional.
- **Log through the context.** `logger.WithContext(ctx)` rather than
  `logger.Log` directly, so request correlation survives across layers.
