package pathoram

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"fmt"
)

// Encryptor provides block encryption and decryption.
// Implementations should be deterministic given the same (blockID, leaf) pair
// to support ORAM access patterns, but must use fresh randomness for security.
type Encryptor interface {
	// Encrypt encrypts plaintext for the given block.
	// The ciphertext includes authentication tag and nonce.
	Encrypt(blockID, leaf int, plaintext []byte) ([]byte, error)

	// Decrypt decrypts ciphertext for the given block.
	Decrypt(blockID, leaf int, ciphertext []byte) ([]byte, error)

	// Overhead returns the number of extra bytes added by encryption
	// (nonce + authentication tag).
	Overhead() int
}

// NoOpEncryptor passes data through without encryption.
// Use only for testing or when encryption is handled externally.
type NoOpEncryptor struct{}

// Encrypt returns a copy of plaintext.
func (NoOpEncryptor) Encrypt(blockID, leaf int, plaintext []byte) ([]byte, error) {
	result := make([]byte, len(plaintext))
	copy(result, plaintext)
	return result, nil
}

// Decrypt returns a copy of ciphertext.
func (NoOpEncryptor) Decrypt(blockID, leaf int, ciphertext []byte) ([]byte, error) {
	result := make([]byte, len(ciphertext))
	copy(result, ciphertext)
	return result, nil
}

// Overhead returns 0 for NoOpEncryptor.
func (NoOpEncryptor) Overhead() int {
	return 0
}

// AESGCMEncryptor provides AES-256-GCM encryption with random nonces.
type AESGCMEncryptor struct {
	aead cipher.AEAD
}

const (
	aesKeySize   = 32 // AES-256
	aesNonceSize = 12 // Standard GCM nonce size
)

// NewAESGCMEncryptor creates a new AES-GCM encryptor with the given 32-byte key.
func NewAESGCMEncryptor(key []byte) (*AESGCMEncryptor, error) {
	if len(key) != aesKeySize {
		return nil, fmt.Errorf("key must be %d bytes, got %d", aesKeySize, len(key))
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create AES cipher: %w", err)
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create GCM: %w", err)
	}

	return &AESGCMEncryptor{aead: aead}, nil
}

// Encrypt encrypts plaintext using AES-GCM with a random nonce.
// Output format: nonce (12 bytes) || ciphertext || tag (16 bytes)
func (e *AESGCMEncryptor) Encrypt(blockID, leaf int, plaintext []byte) ([]byte, error) {
	nonce := make([]byte, aesNonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return nil, ErrEncryptionFailed
	}

	// Use blockID and leaf as additional authenticated data
	aad := makeAAD(blockID, leaf)

	// Seal appends ciphertext+tag to nonce
	ciphertext := e.aead.Seal(nonce, nonce, plaintext, aad)
	return ciphertext, nil
}

// Decrypt decrypts ciphertext using AES-GCM.
// Input format: nonce (12 bytes) || ciphertext || tag (16 bytes)
func (e *AESGCMEncryptor) Decrypt(blockID, leaf int, ciphertext []byte) ([]byte, error) {
	if len(ciphertext) < aesNonceSize+e.aead.Overhead() {
		return nil, ErrDecryptionFailed
	}

	nonce := ciphertext[:aesNonceSize]
	ct := ciphertext[aesNonceSize:]

	aad := makeAAD(blockID, leaf)

	plaintext, err := e.aead.Open(nil, nonce, ct, aad)
	if err != nil {
		return nil, ErrDecryptionFailed
	}

	return plaintext, nil
}

// Overhead returns nonce size + GCM tag size.
func (e *AESGCMEncryptor) Overhead() int {
	return aesNonceSize + e.aead.Overhead()
}

// makeAAD creates additional authenticated data from blockID and leaf.
func makeAAD(blockID, leaf int) []byte {
	aad := make([]byte, 16)
	binary.LittleEndian.PutUint64(aad[0:8], uint64(blockID))
	binary.LittleEndian.PutUint64(aad[8:16], uint64(leaf))
	return aad
}
