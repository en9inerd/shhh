package crypto

import (
	"bytes"
	"testing"
)

func TestEncryptDecrypt(t *testing.T) {
	cs := NewCryptoService()

	passphrase := "secure-passphrase"
	plaintext := []byte("This is a top-secret message.")

	ciphertext, err := cs.Encrypt(plaintext, passphrase)
	if err != nil {
		t.Fatalf("encryption failed: %v", err)
	}

	if bytes.Contains(ciphertext, plaintext) {
		t.Fatal("ciphertext should not contain plaintext")
	}

	decrypted, err := cs.Decrypt(ciphertext, passphrase)
	if err != nil {
		t.Fatalf("decryption failed: %v", err)
	}

	if !bytes.Equal(decrypted, plaintext) {
		t.Errorf("decrypted text does not match original\nExpected: %s\nGot: %s", plaintext, decrypted)
	}
}

func TestDecryptWithWrongPassphrase(t *testing.T) {
	cs := NewCryptoService()

	passphrase := "correct-pass"
	wrongPass := "wrong-pass"
	data := []byte("Secret Message")

	ciphertext, err := cs.Encrypt(data, passphrase)
	if err != nil {
		t.Fatalf("encryption failed: %v", err)
	}

	_, err = cs.Decrypt(ciphertext, wrongPass)
	if err == nil {
		t.Fatal("decryption should fail with wrong passphrase")
	}
}

func TestEncryptEmptyData(t *testing.T) {
	cs := NewCryptoService()
	passphrase := "any"

	ciphertext, err := cs.Encrypt([]byte{}, passphrase)
	if err != nil {
		t.Fatalf("encrypting empty data failed: %v", err)
	}

	plaintext, err := cs.Decrypt(ciphertext, passphrase)
	if err != nil {
		t.Fatalf("decrypting empty data failed: %v", err)
	}

	if len(plaintext) != 0 {
		t.Errorf("expected empty plaintext, got: %s", plaintext)
	}
}

func TestDecryptWithCorruptedCiphertext(t *testing.T) {
	cs := NewCryptoService()
	passphrase := "secret"

	plaintext := []byte("normal input")
	ciphertext, err := cs.Encrypt(plaintext, passphrase)
	if err != nil {
		t.Fatalf("encryption failed: %v", err)
	}

	ciphertext[len(ciphertext)-1] ^= 0xFF

	_, err = cs.Decrypt(ciphertext, passphrase)
	if err == nil {
		t.Fatal("decryption should fail with corrupted ciphertext")
	}
}
