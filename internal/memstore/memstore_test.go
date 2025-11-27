package memstore

import (
	"testing"
	"time"
)

const (
	testPassphrase        = "secret123"
	cleanupDuration       = time.Second
	maxItems              = 10
	maxDataSize     int64 = 1024 // 1KB
)

func newTestStore() *MemoryStore {
	return NewMemoryStore(cleanupDuration, maxItems, maxDataSize)
}

func TestStoreAndRetrieve_Text(t *testing.T) {
	store := newTestStore()
	defer store.Stop()

	data := []byte("Hello, world!")
	ttl := 2 * time.Second

	id, _, err := store.Store(data, "", testPassphrase, ttl)
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	retrieved, filename, err := store.Retrieve(id, testPassphrase)
	if err != nil {
		t.Fatalf("Retrieve failed: %v", err)
	}

	if string(retrieved) != string(data) {
		t.Errorf("Expected %s, got %s", string(data), string(retrieved))
	}

	if filename != "" {
		t.Errorf("Expected empty filename for text, got %s", filename)
	}
}

func TestStoreAndRetrieve_File(t *testing.T) {
	store := newTestStore()
	defer store.Stop()

	data := []byte{0x1f, 0x8b, 0x08}
	filename := "archive.tar.gz"
	ttl := 2 * time.Second

	id, _, err := store.Store(data, filename, testPassphrase, ttl)
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	retrieved, gotFilename, err := store.Retrieve(id, testPassphrase)
	if err != nil {
		t.Fatalf("Retrieve failed: %v", err)
	}

	if string(retrieved) != string(data) {
		t.Errorf("Expected %v, got %v", data, retrieved)
	}

	if gotFilename != filename {
		t.Errorf("Expected filename %s, got %s", filename, gotFilename)
	}
}

func TestStore_InvalidTTL(t *testing.T) {
	store := newTestStore()
	defer store.Stop()

	_, _, err := store.Store([]byte("test"), "", testPassphrase, 0)
	if err == nil || err.Error() != "TTL must be positive" {
		t.Errorf("Expected TTL error, got %v", err)
	}
}

func TestStore_ExceedsDataSize(t *testing.T) {
	store := NewMemoryStore(cleanupDuration, maxItems, 5) // 5 bytes max
	defer store.Stop()

	_, _, err := store.Store([]byte("123456"), "", testPassphrase, 1*time.Second)
	if err == nil || err.Error() != "data size exceeds maximum allowed" {
		t.Errorf("Expected data size error, got %v", err)
	}
}

func TestStore_ExceedsMaxItems(t *testing.T) {
	store := NewMemoryStore(cleanupDuration, 1, maxDataSize) // allow only 1 item
	defer store.Stop()

	_, _, err := store.Store([]byte("one"), "", testPassphrase, 1*time.Second)
	if err != nil {
		t.Fatalf("Unexpected error on first store: %v", err)
	}

	_, _, err = store.Store([]byte("two"), "", testPassphrase, 1*time.Second)
	if err == nil || err.Error() != "memory store is full" {
		t.Errorf("Expected memory full error, got %v", err)
	}
}

func TestRetrieve_Expired(t *testing.T) {
	store := newTestStore()
	defer store.Stop()

	id, _, err := store.Store([]byte("temp data"), "", testPassphrase, 1*time.Millisecond)
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	time.Sleep(10 * time.Millisecond)

	_, _, err = store.Retrieve(id, testPassphrase)
	if err == nil || err.Error() != "item expired" {
		t.Errorf("Expected expiration error, got %v", err)
	}
}

func TestRetrieve_NotFound(t *testing.T) {
	store := newTestStore()
	defer store.Stop()

	_, _, err := store.Retrieve("nonexistent-id", testPassphrase)
	if err == nil || err.Error() != "item not found" {
		t.Errorf("Expected not found error, got %v", err)
	}
}

func TestRetrieve_WrongPassphrase(t *testing.T) {
	store := newTestStore()
	defer store.Stop()

	data := []byte("secret data")
	id, _, err := store.Store(data, "", testPassphrase, 1*time.Second)
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	_, _, err = store.Retrieve(id, "wrongpass")
	if err == nil || err.Error() != "decryption failed" {
		t.Errorf("Expected decryption error, got %v", err)
	}
}

func TestCleaner_RemovesExpired(t *testing.T) {
	store := NewMemoryStore(1*time.Second, maxItems, maxDataSize)
	defer store.Stop()

	id, _, err := store.Store([]byte("clean me"), "", testPassphrase, 1*time.Millisecond)
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	time.Sleep(2 * time.Second)

	store.mu.RLock()
	_, exists := store.items[id]
	store.mu.RUnlock()

	if exists {
		t.Errorf("Expected item to be removed by cleaner")
	}
}
