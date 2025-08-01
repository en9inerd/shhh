package memstore

import (
	"testing"
	"time"
)

const testPassphrase = "secret123"
const cleanupDuration = time.Minute * 5

func TestStoreAndRetrieve_Success(t *testing.T) {
	store := NewMemoryStore(cleanupDuration)
	defer store.Stop()

	data := []byte("Hello, world!")
	isFile := false
	ttl := 2 * time.Second

	id, err := store.Store(data, testPassphrase, isFile, ttl)
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	retrieved, err := store.Retrieve(id, testPassphrase)
	if err != nil {
		t.Fatalf("Retrieve failed: %v", err)
	}

	if string(retrieved) != string(data) {
		t.Errorf("Expected %s, got %s", string(data), string(retrieved))
	}
}

func TestStore_InvalidTTL(t *testing.T) {
	store := NewMemoryStore(cleanupDuration)
	defer store.Stop()

	_, err := store.Store([]byte("test"), testPassphrase, false, 0)
	if err == nil || err.Error() != "TTL must be positive" {
		t.Errorf("Expected TTL error, got %v", err)
	}
}

func TestRetrieve_Expired(t *testing.T) {
	store := NewMemoryStore(cleanupDuration)
	defer store.Stop()

	// Set TTL to 1ms
	id, err := store.Store([]byte("temp data"), testPassphrase, false, 1*time.Millisecond)
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	time.Sleep(10 * time.Millisecond) // Wait for it to expire

	_, err = store.Retrieve(id, testPassphrase)
	if err == nil || err.Error() != "item expired" {
		t.Errorf("Expected expiration error, got %v", err)
	}
}

func TestRetrieve_NotFound(t *testing.T) {
	store := NewMemoryStore(cleanupDuration)
	defer store.Stop()

	_, err := store.Retrieve("nonexistent-id", testPassphrase)
	if err == nil || err.Error() != "item not found" {
		t.Errorf("Expected not found error, got %v", err)
	}
}

func TestRetrieve_WrongPassphrase(t *testing.T) {
	store := NewMemoryStore(cleanupDuration)
	defer store.Stop()

	data := []byte("secret data")
	id, err := store.Store(data, testPassphrase, false, 1*time.Second)
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	_, err = store.Retrieve(id, "wrongpass")
	if err == nil || err.Error() != "decryption failed" {
		t.Errorf("Expected decryption error, got %v", err)
	}
}

func TestCleaner_RemovesExpired(t *testing.T) {
	store := NewMemoryStore(1 * time.Second)
	defer store.Stop()

	id, err := store.Store([]byte("clean me"), testPassphrase, false, 1*time.Millisecond)
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
