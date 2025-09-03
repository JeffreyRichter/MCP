package policies

import (
	"sync"
	"time"
)

// newRateCounter creates a new rate counter
func newRateCounter(d time.Duration) *rateCounter {
	return &rateCounter{duration: d, windowStart: time.Now(), count: 0}
}

// rateCounter tracks the number of requests in the current duration
type rateCounter struct {
	duration    time.Duration // This field is immutable
	mu          sync.Mutex
	windowStart time.Time
	count       int64
}

// Add increments the count
func (rc *rateCounter) Add(delta int64) {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	// Reset if we're in a new duration window
	if now := time.Now(); now.Sub(rc.windowStart) >= rc.duration {
		rc.count, rc.windowStart = 0, now
	}
	rc.count += delta
}

// Rate returns the count in the current time window
func (rc *rateCounter) Rate() int64 {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	if now := time.Now(); now.Sub(rc.windowStart) >= rc.duration {
		rc.count, rc.windowStart = 0, now
	}
	return rc.count
}
