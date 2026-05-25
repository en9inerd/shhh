// Package channel provides an in-memory broadcast queue for encrypted channel messages.
// The server stores and forwards opaque ciphertext blobs; all encryption/decryption
// is performed client-side.
package channel

import (
	"sync"
	"time"
)

// Message is a queued ciphertext blob with server-assigned timestamp.
type Message struct {
	Blob     []byte
	PushedAt time.Time
}

// Subscriber is a per-watcher buffered channel for SSE delivery.
type Subscriber chan Message

// Channel is a single broadcast queue. Methods are safe for concurrent use.
type Channel struct {
	mu           sync.RWMutex
	queue        []Message
	subs         []Subscriber
	maxMsgs      int
	maxWatchers  int
	msgTTL       time.Duration
	lastActivity time.Time
	dynamic      bool // true = created at runtime; eligible for lifetime expiry
}

func newChannel(maxMsgs, maxWatchers int, msgTTL time.Duration, dynamic bool) *Channel {
	return &Channel{
		queue:        make([]Message, 0, maxMsgs),
		maxMsgs:      maxMsgs,
		maxWatchers:  maxWatchers,
		msgTTL:       msgTTL,
		lastActivity: time.Now(),
		dynamic:      dynamic,
	}
}

// Push enqueues blob. Returns (msg, true) on success, (zero, false) if queue full.
func (c *Channel) Push(blob []byte) (Message, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pruneExpiredLocked()
	if len(c.queue) >= c.maxMsgs {
		return Message{}, false
	}
	msg := Message{Blob: blob, PushedAt: time.Now()}
	c.queue = append(c.queue, msg)
	c.lastActivity = msg.PushedAt
	for _, sub := range c.subs {
		select {
		case sub <- msg:
		default:
			// Subscriber buffer full - message will be replayed on drain-on-connect.
		}
	}
	return msg, true
}

func (c *Channel) lastActivityTime() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastActivity
}

// Subscribe atomically checks the watcher cap, registers the subscriber, and
// returns a snapshot of the current non-expired queue. capacity should equal
// ChannelMaxMsgs so the buffer can hold a full queue during drain.
// Returns (nil, nil, false) if the watcher cap is hit.
func (c *Channel) Subscribe(capacity int) (Subscriber, []Message, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.subs) >= c.maxWatchers {
		return nil, nil, false
	}
	sub := make(Subscriber, capacity)
	c.subs = append(c.subs, sub)
	return sub, c.nonExpiredLocked(), true
}

// Unsubscribe removes a previously registered subscriber.
func (c *Channel) Unsubscribe(sub Subscriber) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i, s := range c.subs {
		if s == sub {
			c.subs = append(c.subs[:i], c.subs[i+1:]...)
			return
		}
	}
}

// Pull returns all non-expired messages up to limit (0 = all).
func (c *Channel) Pull(limit int) []Message {
	c.mu.RLock()
	defer c.mu.RUnlock()
	msgs := c.nonExpiredLocked()
	if limit > 0 && len(msgs) > limit {
		msgs = msgs[:limit]
	}
	return msgs
}

func (c *Channel) nonExpiredLocked() []Message {
	now := time.Now()
	out := make([]Message, 0, len(c.queue))
	for _, m := range c.queue {
		if now.Sub(m.PushedAt) < c.msgTTL {
			out = append(out, m)
		}
	}
	return out
}

func (c *Channel) pruneExpiredLocked() {
	now := time.Now()
	i := 0
	for _, m := range c.queue {
		if now.Sub(m.PushedAt) < c.msgTTL {
			c.queue[i] = m
			i++
		}
	}
	c.queue = c.queue[:i]
}

func (c *Channel) pruneExpired() {
	c.mu.Lock()
	c.pruneExpiredLocked()
	c.mu.Unlock()
}

// ChannelStore holds all channels and runs a background cleaner.
type ChannelStore struct {
	mu              sync.RWMutex
	channels        map[string]*Channel
	stopCh          chan struct{}
	msgTTL          time.Duration
	channelLifetime time.Duration
	maxChannels     int
	dynamicCount    int
	maxMsgs         int
	maxWatchers     int
}

// NewChannelStore starts the background cleaner goroutine.
// maxChannels > 0 enables runtime channel creation via Create. channelLifetime=0 falls back to msgTTL.
func NewChannelStore(ids []string, maxMsgs, maxWatchers, maxChannels int, msgTTL, channelLifetime time.Duration) *ChannelStore {
	chs := make(map[string]*Channel, len(ids))
	for _, id := range ids {
		chs[id] = newChannel(maxMsgs, maxWatchers, msgTTL, false)
	}
	if channelLifetime <= 0 {
		channelLifetime = msgTTL
	}
	cs := &ChannelStore{
		channels:        chs,
		stopCh:          make(chan struct{}),
		msgTTL:          msgTTL,
		channelLifetime: channelLifetime,
		maxChannels:     maxChannels,
		maxMsgs:         maxMsgs,
		maxWatchers:     maxWatchers,
	}
	go cs.cleanupLoop()
	return cs
}

// Get returns the Channel for id, or false if not found.
func (cs *ChannelStore) Get(id string) (*Channel, bool) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	ch, ok := cs.channels[id]
	return ch, ok
}

// Create returns false if disabled (maxChannels==0), cap is reached, or id already exists.
func (cs *ChannelStore) Create(id string) bool {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if cs.maxChannels == 0 || cs.dynamicCount >= cs.maxChannels {
		return false
	}
	if _, exists := cs.channels[id]; exists {
		return false
	}
	cs.channels[id] = newChannel(cs.maxMsgs, cs.maxWatchers, cs.msgTTL, true)
	cs.dynamicCount++
	return true
}

// Stop terminates the background cleaner goroutine.
func (cs *ChannelStore) Stop() {
	close(cs.stopCh)
}

func (cs *ChannelStore) cleanupLoop() {
	interval := min(max(cs.msgTTL/60, 30*time.Second), 5*time.Minute)
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			cs.pruneMessages()
			cs.pruneExpiredChannels()
		case <-cs.stopCh:
			return
		}
	}
}

func (cs *ChannelStore) pruneMessages() {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	for _, ch := range cs.channels {
		ch.pruneExpired()
	}
}

// Lock order: cs.mu.Lock → ch.mu.RLock (consistent everywhere; no deadlock risk).
func (cs *ChannelStore) pruneExpiredChannels() {
	now := time.Now()
	cs.mu.Lock()
	defer cs.mu.Unlock()
	for id, ch := range cs.channels {
		if ch.dynamic && now.Sub(ch.lastActivityTime()) > cs.channelLifetime {
			delete(cs.channels, id)
			cs.dynamicCount--
		}
	}
}

// IsValidUUID reports whether id is a 32-character lowercase hex string.
func IsValidUUID(id string) bool {
	if len(id) != 32 {
		return false
	}
	for _, c := range id {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}
