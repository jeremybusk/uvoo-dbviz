package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"strings"
)

const KeyVersion = "v1"

func EncryptString(value string, keyMaterial string) (string, string, error) {
	aead, err := newAEAD(keyMaterial)
	if err != nil {
		return "", "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", "", err
	}
	ciphertext := aead.Seal(nil, nonce, []byte(value), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), base64.StdEncoding.EncodeToString(nonce), nil
}

func DecryptString(ciphertext string, nonce string, keyMaterial string) (string, error) {
	aead, err := newAEAD(keyMaterial)
	if err != nil {
		return "", err
	}
	rawCiphertext, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", err
	}
	rawNonce, err := base64.StdEncoding.DecodeString(nonce)
	if err != nil {
		return "", err
	}
	plaintext, err := aead.Open(nil, rawNonce, rawCiphertext, nil)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

func newAEAD(keyMaterial string) (cipher.AEAD, error) {
	keyMaterial = strings.TrimSpace(keyMaterial)
	if keyMaterial == "" {
		return nil, errors.New("SQVIZ_SECRETS_ENCRYPTION_KEY is required")
	}
	sum := sha256.Sum256([]byte(keyMaterial))
	block, err := aes.NewCipher(sum[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
