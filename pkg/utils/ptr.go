package utils

import "encoding/hex"

func Ptr[T any](v T) *T {
	return &v
}

func BytesToHex(data []byte) string {
	return hex.EncodeToString(data)
}
