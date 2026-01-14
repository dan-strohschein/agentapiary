package secrets

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncryptDecrypt(t *testing.T) {
	passphrase := "test-passphrase-123"
	plaintext := []byte("sensitive-secret-data")

	// Encrypt
	encrypted, err := Encrypt(plaintext, passphrase)
	require.NoError(t, err)
	assert.NotNil(t, encrypted)
	assert.NotEqual(t, plaintext, encrypted)

	// Decrypt
	decrypted, err := Decrypt(encrypted, passphrase)
	require.NoError(t, err)
	assert.Equal(t, plaintext, decrypted)
}

func TestEncrypt_EmptyPassphrase(t *testing.T) {
	plaintext := []byte("test-data")
	_, err := Encrypt(plaintext, "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "passphrase cannot be empty")
}

func TestDecrypt_EmptyPassphrase(t *testing.T) {
	encrypted := []byte("some-encrypted-data")
	_, err := Decrypt(encrypted, "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "passphrase cannot be empty")
}

func TestDecrypt_WrongPassphrase(t *testing.T) {
	passphrase := "correct-passphrase"
	plaintext := []byte("sensitive-data")

	encrypted, err := Encrypt(plaintext, passphrase)
	require.NoError(t, err)

	// Try to decrypt with wrong passphrase
	_, err = Decrypt(encrypted, "wrong-passphrase")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to decrypt")
}

func TestEncryptDecrypt_MultipleValues(t *testing.T) {
	passphrase := "test-passphrase"
	values := [][]byte{
		[]byte("value1"),
		[]byte("value2"),
		[]byte("very-long-value-that-might-cause-issues"),
		[]byte(""),
	}

	for _, plaintext := range values {
		encrypted, err := Encrypt(plaintext, passphrase)
		require.NoError(t, err, "failed to encrypt %q", plaintext)

		decrypted, err := Decrypt(encrypted, passphrase)
		require.NoError(t, err, "failed to decrypt %q", plaintext)
		// Compare using string conversion for better error messages, or compare lengths and contents
		if len(plaintext) == 0 {
			assert.Equal(t, 0, len(decrypted), "decrypted empty value should be empty")
		} else {
			assert.Equal(t, plaintext, decrypted, "decrypted value doesn't match for %q", plaintext)
		}
	}
}
