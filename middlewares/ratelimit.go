package middlewares

import (
	"math"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	apperror "github.com/hris-management-system/go-kit/errors"
	"github.com/hris-management-system/go-kit/logger"
	"github.com/hris-management-system/go-kit/response"
)

// ── configuration ─────────────────────────────────────────────────────────────

// RateLimitConfig describes one rate limit. The zero value is usable: it
// yields the defaults below.
type RateLimitConfig struct {
	// Requests allowed per Window. Defaults to 100.
	Requests int

	// Window over which Requests is counted. Defaults to one minute.
	Window time.Duration

	// Burst is how many requests may arrive back-to-back before the steady
	// rate applies. Defaults to Requests, which lets a client spend its whole
	// allowance at once and then wait — the usual expectation for an API.
	Burst int

	// KeyFunc decides what is being limited. Defaults to the client IP.
	// Override it to limit per user, per tenant, or per device instead.
	KeyFunc func(c *gin.Context) string

	// Skip reports whether a request bypasses the limit entirely — health
	// checks, internal callers. Nil means nothing is skipped.
	Skip func(c *gin.Context) bool

	// IdleTTL is how long an inactive key is retained before eviction.
	// Defaults to max(10*Window, time.Minute). Without eviction the bucket
	// map grows once per distinct key and never shrinks, which is itself a
	// memory-exhaustion vector when keys are attacker-controlled.
	IdleTTL time.Duration

	// SweepInterval is how often eviction runs. Defaults to IdleTTL.
	SweepInterval time.Duration
}

func (cfg *RateLimitConfig) applyDefaults() {
	if cfg.Requests <= 0 {
		cfg.Requests = 100
	}
	if cfg.Window <= 0 {
		cfg.Window = time.Minute
	}
	if cfg.Burst <= 0 {
		cfg.Burst = cfg.Requests
	}
	if cfg.KeyFunc == nil {
		cfg.KeyFunc = func(c *gin.Context) string { return c.ClientIP() }
	}
	if cfg.IdleTTL <= 0 {
		cfg.IdleTTL = 10 * cfg.Window
		if cfg.IdleTTL < time.Minute {
			cfg.IdleTTL = time.Minute
		}
	}
	if cfg.SweepInterval <= 0 {
		cfg.SweepInterval = cfg.IdleTTL
	}
}

// ── limiter ───────────────────────────────────────────────────────────────────

type bucket struct {
	tokens float64
	last   time.Time
}

// RateLimiter is a token bucket per key, held in memory. It is safe for
// concurrent use.
//
// In-memory means per-process: behind N replicas a client gets N times the
// allowance. That is fine for protecting a single instance from overload, and
// not sufficient as a correctness boundary — for a shared quota, back this
// with Redis instead.
type RateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket

	refill float64 // tokens per second
	burst  float64
	cfg    RateLimitConfig

	stop     chan struct{}
	stopOnce sync.Once
}

// NewRateLimiter builds a limiter and starts its eviction sweeper. Call Stop
// when the limiter is discarded, or the sweeper goroutine outlives it.
func NewRateLimiter(cfg RateLimitConfig) *RateLimiter {
	cfg.applyDefaults()

	rl := &RateLimiter{
		buckets: make(map[string]*bucket),
		refill:  float64(cfg.Requests) / cfg.Window.Seconds(),
		burst:   float64(cfg.Burst),
		cfg:     cfg,
		stop:    make(chan struct{}),
	}

	go rl.sweeper()
	return rl
}

// Stop halts the eviction sweeper. Safe to call more than once.
func (rl *RateLimiter) Stop() {
	rl.stopOnce.Do(func() { close(rl.stop) })
}

func (rl *RateLimiter) sweeper() {
	t := time.NewTicker(rl.cfg.SweepInterval)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			rl.evict(time.Now())
		case <-rl.stop:
			return
		}
	}
}

func (rl *RateLimiter) evict(now time.Time) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	for k, b := range rl.buckets {
		if now.Sub(b.last) > rl.cfg.IdleTTL {
			delete(rl.buckets, k)
		}
	}
}

// Allow consumes one token for key. It reports whether the request may
// proceed, how many whole tokens remain, and — when denied — how long until
// one token is available again.
func (rl *RateLimiter) Allow(key string) (ok bool, remaining int, retryAfter time.Duration) {
	return rl.allowAt(key, time.Now())
}

// allowAt is Allow with an injectable clock, so the refill maths is testable
// without sleeping.
func (rl *RateLimiter) allowAt(key string, now time.Time) (bool, int, time.Duration) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	b, seen := rl.buckets[key]
	if !seen {
		// A new key starts full, then immediately spends one token.
		b = &bucket{tokens: rl.burst, last: now}
		rl.buckets[key] = b
	} else {
		// Lazy refill: credit only the time actually elapsed, and never above
		// the burst ceiling.
		elapsed := now.Sub(b.last).Seconds()
		if elapsed > 0 {
			b.tokens = math.Min(rl.burst, b.tokens+elapsed*rl.refill)
			b.last = now
		}
	}

	if b.tokens < 1 {
		deficit := 1 - b.tokens
		return false, 0, time.Duration(deficit / rl.refill * float64(time.Second))
	}

	b.tokens--
	return true, int(b.tokens), 0
}

// ── middleware ────────────────────────────────────────────────────────────────

// RateLimit returns a gin middleware enforcing cfg, and the limiter backing it
// so the caller can Stop it on shutdown or inspect it in tests.
//
//	limiter, rl := middlewares.RateLimit(middlewares.RateLimitConfig{
//	    Requests: 60,
//	    Window:   time.Minute,
//	})
//	defer rl.Stop()
//	r.Use(limiter)
func RateLimit(cfg RateLimitConfig) (gin.HandlerFunc, *RateLimiter) {
	rl := NewRateLimiter(cfg)
	return rl.Middleware(), rl
}

// Middleware enforces this limiter on a gin route or group. Use it directly
// when several routes share one limiter.
func (rl *RateLimiter) Middleware() gin.HandlerFunc {
	limit := strconv.Itoa(rl.cfg.Requests)

	return func(c *gin.Context) {
		if rl.cfg.Skip != nil && rl.cfg.Skip(c) {
			c.Next()
			return
		}

		key := rl.cfg.KeyFunc(c)
		if key == "" {
			// An unidentifiable caller shares one bucket rather than escaping
			// the limit outright.
			key = "-"
		}

		ok, remaining, retryAfter := rl.Allow(key)

		c.Header("X-RateLimit-Limit", limit)
		c.Header("X-RateLimit-Remaining", strconv.Itoa(remaining))

		if ok {
			c.Next()
			return
		}

		// Round up: Retry-After of 0 would invite an immediate retry that is
		// guaranteed to fail again.
		secs := int(math.Ceil(retryAfter.Seconds()))
		if secs < 1 {
			secs = 1
		}
		c.Header("Retry-After", strconv.Itoa(secs))
		c.Header("X-RateLimit-Reset", strconv.FormatInt(time.Now().Add(retryAfter).Unix(), 10))

		if logger.Log != nil {
			logger.SugarWithContext(c.Request.Context()).Warnw("rate limit exceeded",
				"key", key,
				"path", c.Request.URL.Path,
				"method", c.Request.Method,
				"retryAfterSeconds", secs,
			)
		}

		// Reuse the registered code so the body matches every other error the
		// API returns, rather than inventing a second 429 shape.
		response.Err(c, apperror.New(apperror.RateLimited))
		c.Abort()
	}
}

// ── common key functions ──────────────────────────────────────────────────────

// KeyByIP limits per client IP. This is gin's ClientIP, so it honours
// X-Forwarded-For only for proxies you have marked trusted via
// (*gin.Engine).SetTrustedProxies. Leave that unset behind a load balancer and
// every request appears to come from the balancer, collapsing all clients into
// one bucket.
func KeyByIP() func(*gin.Context) string {
	return func(c *gin.Context) string { return c.ClientIP() }
}

// KeyByHeader limits per header value — an API key or device token. Falls back
// to the client IP when the header is absent, so an unauthenticated caller
// cannot dodge the limit by omitting it.
func KeyByHeader(name string) func(*gin.Context) string {
	return func(c *gin.Context) string {
		if v := c.GetHeader(name); v != "" {
			return name + ":" + v
		}
		return "ip:" + c.ClientIP()
	}
}

// KeyByContext limits per value previously stored in the gin context — the
// authenticated user or tenant set by auth middleware. Falls back to the
// client IP when the key is missing or not a string.
func KeyByContext(ctxKey string) func(*gin.Context) string {
	return func(c *gin.Context) string {
		if v, exists := c.Get(ctxKey); exists {
			if s, isStr := v.(string); isStr && s != "" {
				return ctxKey + ":" + s
			}
		}
		return "ip:" + c.ClientIP()
	}
}
