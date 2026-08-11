// Package clock centralizes time access for deterministic business logic.
package clock

import (
	"sync"
	"time"
)

// Clock supplies the current UTC time.
type Clock interface {
	Now() time.Time
}

// Real returns wall-clock UTC time.
type Real struct{}

func (Real) Now() time.Time { return time.Now().UTC() }

// Scaled advances virtual time by scale virtual seconds per real second.
type Scaled struct {
	originReal    time.Time
	originVirtual time.Time
	scale         float64
}

// NewScaled creates a scaled clock anchored at the current wall-clock instant.
func NewScaled(scale float64) *Scaled {
	if scale <= 0 {
		scale = 1
	}
	now := time.Now().UTC()
	return &Scaled{originReal: now, originVirtual: now, scale: scale}
}

func (c *Scaled) Now() time.Time {
	elapsed := time.Since(c.originReal)
	return c.originVirtual.Add(time.Duration(float64(elapsed) * c.scale)).UTC()
}

// Manual is a concurrency-safe test clock.
type Manual struct {
	mu  sync.RWMutex
	now time.Time
}

// NewManual creates a manual clock at the supplied instant.
func NewManual(now time.Time) *Manual { return &Manual{now: now.UTC()} }

func (c *Manual) Now() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.now
}

// Advance moves the manual clock forward.
func (c *Manual) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}
