package server

import (
	"net/http"
	"strconv"
	"sync"
	"time"
)

// rateLimiter allows a fixed number of requests per owner in a sliding window.
type rateLimiter struct {
	mu     sync.Mutex
	limit  int
	window time.Duration
	hits   map[int][]time.Time
	now    func() time.Time
}

func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	return &rateLimiter{limit: limit, window: window, hits: map[int][]time.Time{}, now: time.Now}
}

// allow records a request and reports whether it is within the limit, and if
// not, how long until the next one is allowed.
func (l *rateLimiter) allow(owner int) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	kept := l.hits[owner][:0]
	for _, t := range l.hits[owner] {
		if now.Sub(t) < l.window {
			kept = append(kept, t)
		}
	}
	if len(kept) >= l.limit {
		l.hits[owner] = kept
		return false, l.window - now.Sub(kept[0])
	}
	l.hits[owner] = append(kept, now)
	return true, 0
}

// limited wraps an expensive handler (uploads, transcription) in a rate limit.
func limited(l *rateLimiter, what string, fn apiHandler) apiHandler {
	return func(w http.ResponseWriter, r *http.Request, owner int) error {
		if ok, wait := l.allow(owner); !ok {
			w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
			return problem{429, "Too many " + what + " in a short time. Your history is saved; try again in a few minutes."}
		}
		return fn(w, r, owner)
	}
}

// Per-owner limits for the expensive local readers.
var (
	uploadLimit = newRateLimiter(30, 10*time.Minute)
	voiceLimit  = newRateLimiter(40, 10*time.Minute)
)
