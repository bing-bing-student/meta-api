package utils

import (
	"crypto/rand"
	"fmt"
)

// GenerateRandomBytes 生成指定长度的密码学安全随机字节切片
func GenerateRandomBytes(size int) ([]byte, error) {
	if size <= 0 {
		return nil, fmt.Errorf("invalid size: %d, size must be a positive integer", size)
	}

	b := make([]byte, size)
	if _, err := rand.Read(b); err != nil {
		return nil, fmt.Errorf("failed to generate random bytes: %w", err)
	}

	return b, nil
}
