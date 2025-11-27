package memstore

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	"github.com/en9inerd/shhh/internal/crypto"
)

type StoredItem struct {
	Data      []byte
	CreatedAt time.Time
	ExpiresAt time.Time
	Filename  string // optional
}

type MemoryStore struct {
	items       map[string]*StoredItem
	mu          sync.RWMutex
	crypto      *crypto.CryptoService
	stopCtx     context.Context
	cancel      context.CancelFunc
	maxItems    int
	maxDataSize int64
}

func NewMemoryStore(retention time.Duration, maxItems int, maxDataSize int64) *MemoryStore {
	ctx, cancel := context.WithCancel(context.Background())
	store := &MemoryStore{
		items:       make(map[string]*StoredItem),
		crypto:      crypto.NewCryptoService(),
		stopCtx:     ctx,
		cancel:      cancel,
		maxItems:    maxItems,
		maxDataSize: maxDataSize,
	}
	go store.cleaner(retention)
	return store
}

func generateUUID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return hex.EncodeToString(b), nil
}

func (ms *MemoryStore) Store(data []byte, filename string, passphrase string, ttl time.Duration) (string, *StoredItem, error) {
	if ttl <= 0 {
		return "", nil, errors.New("TTL must be positive")
	}

	if int64(len(data)) > ms.maxDataSize {
		return "", nil, errors.New("data size exceeds maximum allowed")
	}

	ms.mu.RLock()
	if len(ms.items) >= ms.maxItems {
		ms.mu.RUnlock()
		return "", nil, errors.New("memory store is full")
	}
	ms.mu.RUnlock()

	now := time.Now()
	maxTTL := 24 * time.Hour
	if ttl > maxTTL {
		ttl = maxTTL
	}
	expiresAt := now.Add(ttl)

	enc, err := ms.crypto.Encrypt(data, passphrase)
	if err != nil {
		return "", nil, err
	}

	id, err := generateUUID()
	if err != nil {
		return "", nil, err
	}

	item := &StoredItem{
		Data:      enc,
		Filename:  filename,
		CreatedAt: now,
		ExpiresAt: expiresAt,
	}

	ms.mu.Lock()
	if len(ms.items) >= ms.maxItems {
		ms.mu.Unlock()
		return "", nil, errors.New("memory store is full")
	}
	ms.items[id] = item
	ms.mu.Unlock()

	return id, item, nil
}

func (ms *MemoryStore) Retrieve(id, passphrase string) ([]byte, string, error) {
	ms.mu.RLock()
	item, ok := ms.items[id]
	if !ok {
		ms.mu.RUnlock()
		return nil, "", errors.New("item not found")
	}

	if time.Now().After(item.ExpiresAt) {
		ms.mu.RUnlock()
		ms.mu.Lock()
		delete(ms.items, id)
		ms.mu.Unlock()
		return nil, "", errors.New("item expired")
	}

	enc := item.Data
	filename := item.Filename
	ms.mu.RUnlock()

	decrypted, err := ms.crypto.Decrypt(enc, passphrase)
	if err != nil {
		return nil, "", errors.New("decryption failed")
	}

	ms.mu.Lock()
	delete(ms.items, id)
	ms.mu.Unlock()

	return decrypted, filename, nil
}

func (ms *MemoryStore) cleaner(retention time.Duration) {
	ticker := time.NewTicker(retention)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			now := time.Now()
			var expired []string
			for id, item := range ms.items {
				if now.After(item.ExpiresAt) {
					expired = append(expired, id)
				}
			}
			ms.mu.Unlock()

			if len(expired) > 0 {
				ms.mu.Lock()
				for _, id := range expired {
					delete(ms.items, id)
				}
				ms.mu.Unlock()
			}
		case <-ms.stopCtx.Done():
			return
		}
	}
}

func (ms *MemoryStore) Stop() {
	ms.cancel()
}
