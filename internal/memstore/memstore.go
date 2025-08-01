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
	items   map[string]StoredItem
	mu      sync.RWMutex
	crypto  *crypto.CryptoService
	stopCtx context.Context
	cancel  context.CancelFunc
}

func NewMemoryStore(cleanupDuration time.Duration) *MemoryStore {
	ctx, cancel := context.WithCancel(context.Background())
	store := &MemoryStore{
		items:   make(map[string]StoredItem),
		crypto:  crypto.NewCryptoService(),
		stopCtx: ctx,
		cancel:  cancel,
	}
	go store.cleaner(cleanupDuration)
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

func (ms *MemoryStore) Store(data []byte, passphrase string, isFile bool, ttl time.Duration) (string, error) {
	if ttl <= 0 {
		return "", errors.New("TTL must be positive")
	}
	now := time.Now()
	maxTTL := 24 * time.Hour
	if ttl > maxTTL {
		ttl = maxTTL
	}

	expiresAt := now.Add(ttl)

	enc, err := ms.crypto.Encrypt(data, passphrase)
	if err != nil {
		return "", err
	}

	id, err := generateUUID()
	if err != nil {
		return "", err
	}

	ms.mu.Lock()
	ms.items[id] = StoredItem{
		Data:      enc,
		CreatedAt: now,
		ExpiresAt: expiresAt,
		IsFile:    isFile,
	}
	ms.mu.Unlock()

	return id, nil
}

func (ms *MemoryStore) Retrieve(id, passphrase string) ([]byte, error) {
	ms.mu.Lock()
	item, ok := ms.items[id]
	if !ok {
		ms.mu.Unlock()
		return nil, errors.New("item not found")
	}

	if time.Now().After(item.ExpiresAt) {
		delete(ms.items, id)
		ms.mu.Unlock()
		return nil, errors.New("item expired")
	}

	decrypted, err := ms.crypto.Decrypt(item.Data, passphrase)
	if err != nil {
		ms.mu.Unlock()
		return nil, errors.New("decryption failed")
	}

	delete(ms.items, id)
	ms.mu.Unlock()
	return decrypted, nil
}

func (ms *MemoryStore) cleaner(duration time.Duration) {
	ticker := time.NewTicker(duration)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			now := time.Now()
			ms.mu.Lock()
			for id, item := range ms.items {
				if now.After(item.ExpiresAt) {
					delete(ms.items, id)
				}
			}
			ms.mu.Unlock()
		case <-ms.stopCtx.Done():
			return
		}
	}
}

func (ms *MemoryStore) Stop() {
	ms.cancel()
}
