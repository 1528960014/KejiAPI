package gateway

import "sync"

// P7-3: per-key and per-channel (upstream account) in-flight concurrency
// limits. Like ChannelHealth and the rate limiter, the counts are
// per-instance in-memory state (documented trade-off: multi-instance
// deployments each track their own limits).
//
// Semantics:
//   - Begin(keyID, channelID) atomically takes one in-flight slot for both
//     the key and the channel; if either limit would be exceeded it takes
//     nothing and returns false. keyID may be 0 (unknown key, e.g. tests).
//   - A successful dial keeps the slot held for the whole response lifetime;
//     the caller must End(keyID, channelID) when the response finishes.
//   - A failed dial attempt (transport error or retryable status that
//     triggers failover) must End immediately in the dial loop.
type ConcurrencyLimiter struct {
	mu         sync.Mutex
	perKey     int
	perChannel int
	keys       map[int64]int
	channels   map[int64]int
}

func NewConcurrencyLimiter(perKey, perChannel int) *ConcurrencyLimiter {
	return &ConcurrencyLimiter{
		perKey:     perKey,
		perChannel: perChannel,
		keys:       map[int64]int{},
		channels:   map[int64]int{},
	}
}

// Enabled reports whether either limit is active. A nil receiver is always
// disabled, so all methods are nil-safe.
func (c *ConcurrencyLimiter) Enabled() bool {
	return c != nil && (c.perKey > 0 || c.perChannel > 0)
}

// KeyAtLimit reports whether the key already holds perKey in-flight slots.
func (c *ConcurrencyLimiter) KeyAtLimit(keyID int64) bool {
	if c == nil || c.perKey <= 0 || keyID <= 0 {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.keys[keyID] >= c.perKey
}

// ChannelAtLimit reports whether the channel already holds perChannel
// in-flight slots.
func (c *ConcurrencyLimiter) ChannelAtLimit(channelID int64) bool {
	if c == nil || c.perChannel <= 0 || channelID <= 0 {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.channels[channelID] >= c.perChannel
}

// Begin takes one in-flight slot for the key and channel, or neither.
func (c *ConcurrencyLimiter) Begin(keyID, channelID int64) bool {
	if c == nil {
		return true
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.perKey > 0 && keyID > 0 && c.keys[keyID] >= c.perKey {
		return false
	}
	if c.perChannel > 0 && channelID > 0 && c.channels[channelID] >= c.perChannel {
		return false
	}
	if keyID > 0 {
		c.keys[keyID]++
	}
	if channelID > 0 {
		c.channels[channelID]++
	}
	return true
}

// End releases the slot taken by Begin.
func (c *ConcurrencyLimiter) End(keyID, channelID int64) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if keyID > 0 {
		c.keys[keyID]--
		if c.keys[keyID] <= 0 {
			delete(c.keys, keyID)
		}
	}
	if channelID > 0 {
		c.channels[channelID]--
		if c.channels[channelID] <= 0 {
			delete(c.channels, channelID)
		}
	}
}
