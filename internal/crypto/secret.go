// Package crypto provides authenticated symmetric encryption for plugin
// secrets persisted in the database (currently registered_arr.api_key).
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
)

// Seal returns base64(nonce || ciphertext || tag) using AES-256-GCM.
// `key` is any non-empty UTF-8 string; it is hashed with SHA-256 to produce
// the 32-byte AES key.
func Seal(key, plaintext string) (string, error) {
	gcm, err := newGCM(key)
	if err != nil { return "", err }
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil { return "", err }
	ct := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ct), nil
}

// Open is the inverse of Seal. Returns an error if the key is wrong, the
// ciphertext is malformed, or the auth tag fails.
func Open(key, sealed string) (string, error) {
	gcm, err := newGCM(key)
	if err != nil { return "", err }
	raw, err := base64.StdEncoding.DecodeString(sealed)
	if err != nil { return "", err }
	if len(raw) < gcm.NonceSize() { return "", errors.New("crypto: short ciphertext") }
	nonce, ct := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	pt, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil { return "", err }
	return string(pt), nil
}

func newGCM(key string) (cipher.AEAD, error) {
	sum := sha256.Sum256([]byte(key))
	block, err := aes.NewCipher(sum[:])
	if err != nil { return nil, err }
	return cipher.NewGCM(block)
}
