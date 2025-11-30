package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"

	"golang.org/x/crypto/argon2"
)

type CryptoService struct {
	SaltSize   int
	NonceSize  int
	KeyLength  uint32
	Memory     uint32 // in KB (e.g., 64*1024 = 64MB)
	Iterations uint32
	Threads    uint8
}

func NewCryptoService() *CryptoService {
	return &CryptoService{
		SaltSize:   16,
		NonceSize:  12,
		KeyLength:  32,        // AES-256
		Memory:     64 * 1024, // 64 MB
		Iterations: 3,
		Threads:    4,
	}
}

func (cs *CryptoService) deriveKey(passphrase string, salt []byte) []byte {
	return argon2.IDKey([]byte(passphrase), salt, cs.Iterations, cs.Memory, cs.Threads, cs.KeyLength)
}

func (cs *CryptoService) Encrypt(data []byte, passphrase string) ([]byte, error) {
	salt := make([]byte, cs.SaltSize)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}

	key := cs.deriveKey(passphrase, salt)

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, cs.NonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}

	ciphertext := gcm.Seal(nil, nonce, data, nil)

	result := append(salt, nonce...)
	result = append(result, ciphertext...)
	return result, nil
}

func (cs *CryptoService) Decrypt(data []byte, passphrase string) ([]byte, error) {
	if len(data) < cs.SaltSize+cs.NonceSize {
		return nil, errors.New("ciphertext too short")
	}

	salt := data[:cs.SaltSize]
	nonce := data[cs.SaltSize : cs.SaltSize+cs.NonceSize]
	ciphertext := data[cs.SaltSize+cs.NonceSize:]

	key := cs.deriveKey(passphrase, salt)

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	return gcm.Open(nil, nonce, ciphertext, nil)
}
