package middleware

import (
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"golang.org/x/time/rate"
)

type rateLimiter struct {
	mu       sync.Mutex
	entries  map[string]*limiterEntry
	rps      rate.Limit
	burst    int
}

type limiterEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

func NewRateLimiter(rps float64, burst int) *rateLimiter {
	rl := &rateLimiter{
		entries: make(map[string]*limiterEntry),
		rps:     rate.Limit(rps),
		burst:   burst,
	}
	go rl.cleanupLoop()
	return rl
}

// cleanupLoop removes stale per-user limiters every 5 minutes.
func (rl *rateLimiter) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		rl.mu.Lock()
		for id, e := range rl.entries {
			if time.Since(e.lastSeen) > 10*time.Minute {
				delete(rl.entries, id)
			}
		}
		rl.mu.Unlock()
	}
}

func (rl *rateLimiter) get(userID string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	e, ok := rl.entries[userID]
	if !ok {
		e = &limiterEntry{limiter: rate.NewLimiter(rl.rps, rl.burst)}
		rl.entries[userID] = e
	}
	e.lastSeen = time.Now()
	return e.limiter
}

// Middleware returns a Fiber handler that enforces per-user rate limits.
// Requires JWTMiddleware to have run first (sets c.Locals("userID")).
func (rl *rateLimiter) Middleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		userID, _ := c.Locals("userID").(string)
		if userID == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
		}
		if !rl.get(userID).Allow() {
			c.Set("Retry-After", "60")
			return c.Status(fiber.StatusTooManyRequests).
				JSON(fiber.Map{"error": "rate limit exceeded", "retry_after": 60})
		}
		return c.Next()
	}
}
