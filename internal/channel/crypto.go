package channel

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"

	"golang.org/x/crypto/pbkdf2"
)

const (
	versionPBKDF2SHA256 = byte(0x01)
	saltSize            = 16
	nonceSize           = 12
	pbkdf2Iterations    = 600_000
	keySize             = 32

	// MinBlobSize = version(1) + salt(16) + nonce(12) + min_inner(18) + gcm_tag(16) = 63.
	// min_inner = msg_id(16) + type(1) + sender_name_len(1).
	MinBlobSize  = 63
	MinInnerSize = 18 // msg_id(16) + type(1) + sender_name_len(1)
)

// EncryptBlob builds: [version(1) | salt(16) | nonce(12) | ciphertext+tag].
// channelUUID must be the 32-character ASCII hex UUID used as AAD.
// salt and nonce are freshly generated per call via crypto/rand.
func EncryptBlob(plaintext []byte, passphrase, channelUUID string) ([]byte, error) {
	salt := make([]byte, saltSize)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, fmt.Errorf("rand salt: %w", err)
	}
	nonce := make([]byte, nonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("rand nonce: %w", err)
	}

	key := pbkdf2.Key([]byte(passphrase), salt, pbkdf2Iterations, keySize, sha256.New)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	// AAD = channelUUID as 32 ASCII hex bytes.
	ct := gcm.Seal(nil, nonce, plaintext, []byte(channelUUID))

	blob := make([]byte, 0, 1+saltSize+nonceSize+len(ct))
	blob = append(blob, versionPBKDF2SHA256)
	blob = append(blob, salt...)
	blob = append(blob, nonce...)
	blob = append(blob, ct...)
	return blob, nil
}

// DecryptBlob reverses EncryptBlob.
func DecryptBlob(blob []byte, passphrase, channelUUID string) ([]byte, error) {
	if len(blob) < MinBlobSize {
		return nil, errors.New("blob too short")
	}
	if blob[0] != versionPBKDF2SHA256 {
		return nil, fmt.Errorf("unknown version: 0x%02x", blob[0])
	}
	salt := blob[1 : 1+saltSize]
	nonce := blob[1+saltSize : 1+saltSize+nonceSize]
	ct := blob[1+saltSize+nonceSize:]

	key := pbkdf2.Key([]byte(passphrase), salt, pbkdf2Iterations, keySize, sha256.New)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	return gcm.Open(nil, nonce, ct, []byte(channelUUID))
}
