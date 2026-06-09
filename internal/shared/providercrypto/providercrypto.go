package providercrypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

const (
	keySize   = 32
	nonceSize = 12
	tagSize   = 16
)

var (
	ErrKeySize    = fmt.Errorf("providercrypto: key must be %d bytes", keySize)
	ErrCiphertext = errors.New("providercrypto: ciphertext too short")
)

func ParseKey(encoded string) ([]byte, error) {
	key, err := decode(encoded)
	if err != nil {
		return nil, fmt.Errorf("providercrypto: decode key: %w", err)
	}
	if len(key) != keySize {
		return nil, ErrKeySize
	}
	return key, nil
}

func Encrypt(plaintext string, key []byte) (string, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, nonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("providercrypto: nonce: %w", err)
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

func Decrypt(encoded string, key []byte) (string, error) {
	raw, err := decode(encoded)
	if err != nil {
		return "", fmt.Errorf("providercrypto: decode ciphertext: %w", err)
	}
	gcm, err := newGCM(key)
	if err != nil {
		return "", err
	}
	if len(raw) < nonceSize+tagSize {
		return "", ErrCiphertext
	}
	nonce, sealed := raw[:nonceSize], raw[nonceSize:]
	plain, err := gcm.Open(nil, nonce, sealed, nil)
	if err != nil {
		return "", fmt.Errorf("providercrypto: open: %w", err)
	}
	return string(plain), nil
}

func newGCM(key []byte) (cipher.AEAD, error) {
	if len(key) != keySize {
		return nil, ErrKeySize
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("providercrypto: cipher: %w", err)
	}
	return cipher.NewGCM(block)
}

func decode(s string) ([]byte, error) {
	if b, err := base64.StdEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	return base64.RawStdEncoding.DecodeString(s)
}
