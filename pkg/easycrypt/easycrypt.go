package easycrypt

// All credit to
// https://tutorialedge.net/golang/go-encrypt-decrypt-aes-tutorial/

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"

	"github.com/pkg/errors"
)

// Encrypt uses aes encryption on text using key
func Encrypt(bytes []byte, key string) ([]byte, error) {

	// generate a new aes cipher using our 32 byte long key
	c, err := aes.NewCipher([]byte(key))

	// if there are any errors, handle them
	if err != nil {
		return []byte{}, errors.Wrap(err, "easycrypt: New cipher issue")
	}

	// gcm or Galois/Counter Mode, is a mode of operation
	// for symmetric key cryptographic block ciphers
	// - https://en.wikipedia.org/wiki/Galois/Counter_Mode
	gcm, err := cipher.NewGCM(c)

	// if any error generating new GCM handle them
	if err != nil {
		return []byte{}, errors.Wrap(err, "easycrypt: cipher.NewGCM issue")
	}

	// creates a new byte array the size of the nonce
	// which must be passed to Seal
	nonce := make([]byte, gcm.NonceSize())

	// populates our nonce with a cryptographically secure
	// random sequence
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return []byte{}, errors.Wrap(err, "easycrypt: Nonce issue")
	}

	// here we encrypt our text using the Seal function
	// Seal encrypts and authenticates plaintext, authenticates the
	// additional data and appends the result to dst, returning the updated
	// slice. The nonce must be NonceSize() bytes long and unique for all
	// time, for a given key.
	// the WriteFile method returns an error if unsuccessful
	return gcm.Seal(nonce, nonce, bytes, nil), nil
}

// Decrypt decrypts bytes using key (aes)
func Decrypt(bytes []byte, key string) ([]byte, error) {

	c, err := aes.NewCipher([]byte(key))
	if err != nil {
		return []byte{}, errors.Wrap(err, "easycrypt: New cipher issue")
	}

	gcm, err := cipher.NewGCM(c)
	if err != nil {
		return []byte{}, errors.Wrap(err, "easycrypt: cipher.NewGCM issue")
	}

	nonceSize := gcm.NonceSize()
	if len(bytes) < nonceSize {
		return []byte{}, errors.New(fmt.Sprintf("easycrypt: Nonce issue: len(bytes)(%v) < nonceSize(%v)", len(bytes), nonceSize))
	}

	nonce, bytes := bytes[:nonceSize], bytes[nonceSize:]
	plain, err := gcm.Open(nil, nonce, bytes, nil)
	if err != nil {
		return []byte{}, errors.Wrap(err, "easycrypt: gcm.Open issue")
	}
	return plain, nil
}
