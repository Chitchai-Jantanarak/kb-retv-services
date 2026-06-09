package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"

	"github.com/my/app/pkg/constant/configs"
)

func EncryptAES(plaintext string) (*string, error) {
	block, err := aes.NewCipher([]byte(configs.AES_KEY))
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, 12, 12+len(plaintext)+16)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	ciphertext := aesGCM.Seal(nil, nonce, []byte(plaintext), nil)
	result := hex.EncodeToString(nonce) + hex.EncodeToString(ciphertext)
	return &result, nil
}

func EncryptAESByte(plaintext string) ([]byte, error) {
	block, err := aes.NewCipher([]byte(configs.AES_KEY))
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, 12)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	ciphertext := aesGCM.Seal(nonce, nonce, []byte(plaintext), nil)
	return ciphertext, nil
}

func DecryptAESByte(encrypted []byte) (*string, error) {
	if len(encrypted) < 12 {
		return nil, fmt.Errorf("invalid encrypted data")
	}
	nonce := encrypted[:12]
	ciphertext := encrypted[12:]
	block, err := aes.NewCipher([]byte(configs.AES_KEY))
	if err != nil {
		return nil, err
	}
	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	plaintext, err := aesGCM.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}
	result := string(plaintext)
	return &result, nil
}

func DecryptAES(encrypted string) (*string, error) {
	data, err := hex.DecodeString(encrypted)
	if err != nil {
		return nil, err
	}
	if len(data) < 12 {
		return nil, fmt.Errorf("invalid encrypted data")
	}
	nonce := data[:12]
	ciphertext := data[12:]
	block, err := aes.NewCipher([]byte(configs.AES_KEY))
	if err != nil {
		return nil, err
	}
	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	plaintext, err := aesGCM.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}
	result := string(plaintext)
	return &result, nil
}
