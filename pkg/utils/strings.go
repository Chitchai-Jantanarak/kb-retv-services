package utils

import (
	"fmt"
	"math/rand"
	"time"
)

var seededRand *rand.Rand = rand.New(rand.NewSource(time.Now().UnixNano()))

func CheckNullorZero(value interface{}) bool {
	switch v := value.(type) {
	case string:
		return v == ""
	case int, int8, int16, int32, int64:
		return v == 0
	case uint, uint8, uint16, uint32, uint64:
		return v == 0
	case *string:
		return v == nil
	case *int, *int8, *int16, *int32, *int64:
		return v == nil
	case *uint, *uint8, *uint16, *uint32, *uint64:
		return v == nil
	}
	return false
}

func StringPtrFrom(data map[string]interface{}, key string) *string {
	val, ok := data[key]
	if !ok || val == nil {
		return nil
	}

	var strVal string
	switch v := val.(type) {
	case string:
		if v == "" {
			return nil
		}
		strVal = v
	case float64, float32, int, int64, int32, uint, uint64:
		strVal = fmt.Sprintf("%v", v)
	default:
		return nil
	}

	return &strVal
}

func RandomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*()-_=+[]{}|;:,.<>?/~"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[seededRand.Intn(len(letters))]
	}
	return string(b)
}

func StringPtr(s string) *string {
	return &s
}
