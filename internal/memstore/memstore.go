package memstore

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/en9inerd/shhh/internal/crypto"
	"github.com/en9inerd/shhh/internal/util"
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
	logger      *slog.Logger
}

func NewMemoryStore(logger *slog.Logger, retention time.Duration, maxItems int, maxDataSize int64) *MemoryStore {
	ctx, cancel := context.WithCancel(context.Background())
	store := &MemoryStore{
		items:       make(map[string]*StoredItem),
		crypto:      crypto.NewCryptoService(),
		stopCtx:     ctx,
		cancel:      cancel,
		maxItems:    maxItems,
		maxDataSize: maxDataSize,
		logger:      logger,
	}
	go store.cleaner(retention)
	return store
}

func (ms *MemoryStore) Store(data []byte, filename string, passphrase string, ttl time.Duration) (string, *StoredItem, error) {
	if ttl <= 0 {
		return "", nil, errors.New("TTL must be positive")
	}

	if int64(len(data)) > ms.maxDataSize {
		return "", nil, errors.New("data size exceeds maximum allowed")
	}

	filename = util.SanitizeFilename(filename)

	ms.mu.RLock()
	if len(ms.items) >= ms.maxItems {
		ms.mu.RUnlock()
		ms.logger.Warn("memory store is full", "max_items", ms.maxItems)
		return "", nil, errors.New("memory store is full")
	}
	ms.mu.RUnlock()

	now := time.Now()
	expiresAt := now.Add(ttl)

	enc, err := ms.crypto.Encrypt(data, passphrase)
	if err != nil {
		return "", nil, err
	}

	id, err := util.GenerateID()
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
	defer ms.mu.Unlock()

	if len(ms.items) >= ms.maxItems {
		ms.logger.Warn("memory store is full", "max_items", ms.maxItems)
		return "", nil, errors.New("memory store is full")
	}

	ms.items[id] = item
	return id, item, nil
}

func (ms *MemoryStore) Retrieve(id, passphrase string) ([]byte, string, error) {
	ms.mu.Lock()
	item, ok := ms.items[id]
	if !ok {
		ms.mu.Unlock()
		return nil, "", errors.New("item not found")
	}

	if time.Now().After(item.ExpiresAt) {
		delete(ms.items, id)
		ms.mu.Unlock()
		ms.logger.Debug("item expired on retrieval", "id", id)
		return nil, "", errors.New("item expired")
	}

	delete(ms.items, id)
	ms.mu.Unlock()

	decrypted, err := ms.crypto.Decrypt(item.Data, passphrase)
	if err != nil {
		return nil, "", errors.New("decryption failed")
	}

	return decrypted, item.Filename, nil
}

func (ms *MemoryStore) cleaner(retention time.Duration) {
	interval := retention / 60
	switch {
	case retention < time.Minute:
		interval = retention
	case interval < 30*time.Second:
		interval = 30 * time.Second
	case interval > 5*time.Minute:
		interval = 5 * time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			now := time.Now()
			var expired []string
			ms.mu.RLock()
			for id, item := range ms.items {
				if now.After(item.ExpiresAt) {
					expired = append(expired, id)
				}
			}
			ms.mu.RUnlock()

			if len(expired) > 0 {
				ms.mu.Lock()
				for _, id := range expired {
					delete(ms.items, id)
				}
				ms.mu.Unlock()
				ms.logger.Debug("cleaned expired items", "count", len(expired))
			}
		case <-ms.stopCtx.Done():
			return
		}
	}
}

func (ms *MemoryStore) Stop() {
	ms.cancel()
}
