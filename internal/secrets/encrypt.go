// Package secrets provides secret encryption and decryption functionality.
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"

	"golang.org/x/crypto/pbkdf2"
)

const (
	// saltSize is the size of the salt in bytes.
	saltSize = 16
	// nonceSize is the size of the nonce for GCM in bytes.
	nonceSize = 12
	// keySize is the size of the derived key in bytes (AES-256).
	keySize = 32
	// iterations is the number of PBKDF2 iterations.
	iterations = 100000
)

// Encrypt encrypts data using AES-256-GCM with a passphrase.
// The format is: base64(salt + nonce + ciphertext + tag)
func Encrypt(data []byte, passphrase string) ([]byte, error) {
	if len(passphrase) == 0 {
		return nil, fmt.Errorf("passphrase cannot be empty")
	}

	// Generate salt
	salt := make([]byte, saltSize)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, fmt.Errorf("failed to generate salt: %w", err)
	}

	// Derive key from passphrase
	key := pbkdf2.Key([]byte(passphrase), salt, iterations, keySize, sha256.New)

	// Create cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	// Create GCM
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	// Generate nonce
	nonce := make([]byte, nonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Encrypt
	ciphertext := gcm.Seal(nonce, nonce, data, nil)

	// Combine salt + ciphertext
	result := append(salt, ciphertext...)

	// Base64 encode
	encoded := make([]byte, base64.StdEncoding.EncodedLen(len(result)))
	base64.StdEncoding.Encode(encoded, result)

	return encoded, nil
}

// Decrypt decrypts data encrypted with Encrypt.
func Decrypt(encrypted []byte, passphrase string) ([]byte, error) {
	if len(passphrase) == 0 {
		return nil, fmt.Errorf("passphrase cannot be empty")
	}

	// Base64 decode
	decoded := make([]byte, base64.StdEncoding.DecodedLen(len(encrypted)))
	n, err := base64.StdEncoding.Decode(decoded, encrypted)
	if err != nil {
		return nil, fmt.Errorf("failed to decode base64: %w", err)
	}
	decoded = decoded[:n]

	if len(decoded) < saltSize+nonceSize {
		return nil, fmt.Errorf("encrypted data too short")
	}

	// Extract salt and ciphertext
	salt := decoded[:saltSize]
	ciphertext := decoded[saltSize:]

	// Derive key from passphrase
	key := pbkdf2.Key([]byte(passphrase), salt, iterations, keySize, sha256.New)

	// Create cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	// Create GCM
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	// Extract nonce
	nonce := ciphertext[:nonceSize]
	ciphertext = ciphertext[nonceSize:]

	// Decrypt
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt: %w", err)
	}

	return plaintext, nil
}
