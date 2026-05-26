package middleware

import (
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

const (
	defaultRatePerMinute = 100
	// cleanupInterval controls how often idle entries are pruned.
	cleanupInterval = 5 * time.Minute
	// idleTimeout removes limiter entries that haven't been seen recently.
	idleTimeout = 10 * time.Minute
)

type ipEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// RateLimiter is an in-memory, per-IP token bucket limiter.
type RateLimiter struct {
	mu      sync.Mutex
	entries map[string]*ipEntry
	rps     rate.Limit
	burst   int
}

// NewRateLimiter constructs a RateLimiter with requestsPerMinute tokens
// available per IP per minute. The burst is set to requestsPerMinute to allow
// short spikes.
func NewRateLimiter(requestsPerMinute int) *RateLimiter {
	if requestsPerMinute <= 0 {
		requestsPerMinute = defaultRatePerMinute
	}
	rl := &RateLimiter{
		entries: make(map[string]*ipEntry),
		rps:     rate.Limit(float64(requestsPerMinute) / 60.0),
		burst:   requestsPerMinute,
	}
	go rl.cleanupLoop()
	return rl
}

// Middleware returns a handler that enforces the rate limit per source IP.
// Requests that exceed the limit receive a 429.
func (rl *RateLimiter) Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := extractSourceIPString(r)
			if !rl.allow(ip) {
				http.Error(w, `{"error":"rate limit exceeded"}`, http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func (rl *RateLimiter) allow(ip string) bool {
	rl.mu.Lock()
	entry, ok := rl.entries[ip]
	if !ok {
		entry = &ipEntry{
			limiter: rate.NewLimiter(rl.rps, rl.burst),
		}
		rl.entries[ip] = entry
	}
	entry.lastSeen = time.Now()
	allowed := entry.limiter.Allow()
	rl.mu.Unlock()
	return allowed
}

func (rl *RateLimiter) cleanupLoop() {
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()
	for range ticker.C {
		rl.mu.Lock()
		cutoff := time.Now().Add(-idleTimeout)
		for ip, e := range rl.entries {
			if e.lastSeen.Before(cutoff) {
				delete(rl.entries, ip)
			}
		}
		rl.mu.Unlock()
	}
}

func extractSourceIPString(r *http.Request) string {
	ip := extractSourceIP(r)
	if ip == nil {
		return r.RemoteAddr
	}
	return ip.String()
}
