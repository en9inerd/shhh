package channel

import (
	"bytes"
	"testing"
	"time"
)

// --- crypto tests ---

// minEnvelope returns a valid minimum envelope (18 bytes: msg_id + type + name_len).
func minEnvelope() []byte {
	env := make([]byte, MinInnerSize)
	env[16] = 0x01 // type = text
	env[17] = 0x00 // sender_name_len = 0
	return env
}

func TestEncryptDecryptRoundtrip(t *testing.T) {
	uuid := "a1b2c3d4e5f678901234567890abcdef"
	plain := append(minEnvelope(), []byte("hello, world")...)
	blob, err := EncryptBlob(plain, "s3cret", uuid)
	if err != nil {
		t.Fatalf("EncryptBlob: %v", err)
	}
	if len(blob) < MinBlobSize {
		t.Fatalf("blob too short: %d (need %d)", len(blob), MinBlobSize)
	}

	got, err := DecryptBlob(blob, "s3cret", uuid)
	if err != nil {
		t.Fatalf("DecryptBlob: %v", err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatalf("plaintext mismatch: got %q want %q", got, plain)
	}
}

func TestDecryptWrongPassphrase(t *testing.T) {
	uuid := "a1b2c3d4e5f678901234567890abcdef"
	blob, _ := EncryptBlob(minEnvelope(), "correct", uuid)
	_, err := DecryptBlob(blob, "wrong", uuid)
	if err == nil {
		t.Fatal("expected error with wrong passphrase")
	}
}

func TestDecryptWrongUUID(t *testing.T) {
	uuid1 := "a1b2c3d4e5f678901234567890abcdef"
	uuid2 := "ffffffffffffffffffffffffffffffff"
	blob, _ := EncryptBlob(minEnvelope(), "pass", uuid1)
	_, err := DecryptBlob(blob, "pass", uuid2)
	if err == nil {
		t.Fatal("expected error with wrong UUID (AAD mismatch)")
	}
}

func TestDecryptUnknownVersion(t *testing.T) {
	blob := make([]byte, MinBlobSize)
	blob[0] = 0x02 // unknown version
	_, err := DecryptBlob(blob, "pass", "a1b2c3d4e5f678901234567890abcdef")
	if err == nil {
		t.Fatal("expected error for unknown version")
	}
}

func TestDecryptTooShort(t *testing.T) {
	_, err := DecryptBlob(make([]byte, 10), "pass", "a1b2c3d4e5f678901234567890abcdef")
	if err == nil {
		t.Fatal("expected error for short blob")
	}
}

func TestFreshSaltPerCall(t *testing.T) {
	uuid := "a1b2c3d4e5f678901234567890abcdef"
	b1, _ := EncryptBlob(minEnvelope(), "p", uuid)
	b2, _ := EncryptBlob(minEnvelope(), "p", uuid)
	// salts (bytes 1..16) must differ
	if bytes.Equal(b1[1:17], b2[1:17]) {
		t.Fatal("same salt used twice - nonce reuse risk")
	}
}

// --- IsValidUUID tests ---

func TestIsValidUUID(t *testing.T) {
	cases := []struct {
		id   string
		want bool
	}{
		{"a1b2c3d4e5f678901234567890abcdef", true},
		{"00000000000000000000000000000000", true},
		{"ffffffffffffffffffffffffffffffff", true},
		{"A1B2C3D4E5F678901234567890ABCDEF", false},  // uppercase
		{"a1b2c3d4e5f678901234567890abcde", false},   // 31 chars
		{"a1b2c3d4e5f678901234567890abcdeff", false}, // 33 chars
		{"a1b2c3d4e5f678901234567890abcdeg", false},  // 'g' invalid
		{"", false},
	}
	for _, tc := range cases {
		if got := IsValidUUID(tc.id); got != tc.want {
			t.Errorf("IsValidUUID(%q) = %v, want %v", tc.id, got, tc.want)
		}
	}
}

// --- Channel/ChannelStore tests ---

func TestChannelPushPull(t *testing.T) {
	ch := newChannel(5, 10, time.Hour)
	blob := []byte("ciphertext")
	msg, ok := ch.Push(blob)
	if !ok {
		t.Fatal("Push failed")
	}
	if !bytes.Equal(msg.Blob, blob) {
		t.Fatal("Blob mismatch")
	}

	msgs := ch.Pull(0)
	if len(msgs) != 1 {
		t.Fatalf("Pull: got %d msgs, want 1", len(msgs))
	}
	if !bytes.Equal(msgs[0].Blob, blob) {
		t.Fatal("Pulled blob mismatch")
	}
}

func TestChannelQueueFull(t *testing.T) {
	ch := newChannel(2, 10, time.Hour)
	ch.Push([]byte("a"))
	ch.Push([]byte("b"))
	_, ok := ch.Push([]byte("c"))
	if ok {
		t.Fatal("expected queue full")
	}
}

func TestChannelSubscribeBroadcast(t *testing.T) {
	ch := newChannel(10, 5, time.Hour)
	sub, snapshot, ok := ch.Subscribe(10)
	if !ok {
		t.Fatal("Subscribe failed")
	}
	if len(snapshot) != 0 {
		t.Fatal("expected empty snapshot")
	}

	ch.Push([]byte("hello"))

	select {
	case msg := <-sub:
		if !bytes.Equal(msg.Blob, []byte("hello")) {
			t.Fatal("wrong message")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for message")
	}
	ch.Unsubscribe(sub)
}

func TestChannelSubscribeSnapshot(t *testing.T) {
	ch := newChannel(10, 5, time.Hour)
	ch.Push([]byte("queued"))

	_, snapshot, ok := ch.Subscribe(10)
	if !ok {
		t.Fatal("Subscribe failed")
	}
	if len(snapshot) != 1 || !bytes.Equal(snapshot[0].Blob, []byte("queued")) {
		t.Fatalf("unexpected snapshot: %v", snapshot)
	}
}

func TestChannelWatcherCap(t *testing.T) {
	ch := newChannel(10, 2, time.Hour)
	sub1, _, ok := ch.Subscribe(10)
	if !ok {
		t.Fatal("first Subscribe failed")
	}
	sub2, _, ok := ch.Subscribe(10)
	if !ok {
		t.Fatal("second Subscribe failed")
	}
	_, _, ok = ch.Subscribe(10)
	if ok {
		t.Fatal("third Subscribe should fail (cap=2)")
	}
	ch.Unsubscribe(sub1)
	ch.Unsubscribe(sub2)
}

func TestChannelTTLExpiry(t *testing.T) {
	ch := newChannel(10, 5, 10*time.Millisecond)
	ch.Push([]byte("expires"))
	time.Sleep(20 * time.Millisecond)
	msgs := ch.Pull(0)
	if len(msgs) != 0 {
		t.Fatalf("expected 0 msgs after TTL, got %d", len(msgs))
	}
}

func TestChannelPullWithLimit(t *testing.T) {
	ch := newChannel(10, 5, time.Hour)
	ch.Push([]byte("a"))
	ch.Push([]byte("b"))
	ch.Push([]byte("c"))
	msgs := ch.Pull(2)
	if len(msgs) != 2 {
		t.Fatalf("Pull(2) of 3 msgs = %d, want 2", len(msgs))
	}
	// Pull(0) means no limit - return all.
	all := ch.Pull(0)
	if len(all) != 3 {
		t.Fatalf("Pull(0) = %d, want 3", len(all))
	}
}

func TestChannelPruneExpired(t *testing.T) {
	ch := newChannel(10, 5, 10*time.Millisecond)
	ch.Push([]byte("expires"))
	time.Sleep(20 * time.Millisecond)
	ch.pruneExpired() // explicit call to the otherwise-cleanup-only path
	ch.mu.RLock()
	n := len(ch.queue)
	ch.mu.RUnlock()
	if n != 0 {
		t.Fatalf("queue after pruneExpired = %d, want 0", n)
	}
}

func TestChannelUnsubscribeThenPush(t *testing.T) {
	ch := newChannel(10, 5, time.Hour)
	sub, _, _ := ch.Subscribe(10)
	ch.Unsubscribe(sub)
	// Push after Unsubscribe must not deadlock or panic.
	_, ok := ch.Push([]byte("hello"))
	if !ok {
		t.Fatal("Push after Unsubscribe should succeed")
	}
}

func TestChannelStoreGetUnknown(t *testing.T) {
	cs := NewChannelStore([]string{"a1b2c3d4e5f678901234567890abcdef"}, 20, 10, time.Hour)
	defer cs.Stop()

	_, ok := cs.Get("00000000000000000000000000000000")
	if ok {
		t.Fatal("expected not found for unknown UUID")
	}

	_, ok = cs.Get("a1b2c3d4e5f678901234567890abcdef")
	if !ok {
		t.Fatal("expected found for known UUID")
	}
}
