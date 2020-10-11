package easycrypt

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEncryptDecrypt(t *testing.T) {
	originalBytes := []byte("This is the test string we are encrypting/decrypting")
	key := "ThisIsMy32BytesKeyForTestingFine"
	encryptedBytes, err := Encrypt(originalBytes, key)
	assert.Equal(t, err, nil, "Failed to Encrypt")

	copyOfBytes, err := Decrypt(encryptedBytes, key)
	assert.Equal(t, err, nil, "Failed to Decrypt")
	assert.Equal(t, originalBytes, copyOfBytes, "Encrypt / Decrypt corrupted the testStr")
}
