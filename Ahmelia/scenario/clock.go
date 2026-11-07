package scenario

import (
	"sync"
	"time"
)

// Clock is the simulated time source for a scene. Decisions receive time from
// this clock; they never call time.Now.
type Clock struct {
	mu  sync.Mutex
	now time.Time
	loc *time.Location
}

// NewClock starts a clock at at in loc.
func NewClock(at time.Time, loc *time.Location) *Clock {
	if loc == nil {
		loc = at.Location()
	}
	return &Clock{now: at.In(loc), loc: loc}
}

// Now returns the current simulated instant.
func (c *Clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// At jumps to an absolute instant.
func (c *Clock) At(t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = t.In(c.loc)
}

// Advance moves the clock forward.
func (c *Clock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// Location returns the scene timezone.
func (c *Clock) Location() *time.Location {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.loc
}

// EndOfDay returns the first instant of the next local day.
func (c *Clock) EndOfDay() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	y, m, d := c.now.In(c.loc).Date()
	return time.Date(y, m, d+1, 0, 0, 0, 0, c.loc)
}
