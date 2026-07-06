package idutil

import (
	"fmt"
	"strconv"
)

// ParseID 将 string 形态的雪花 ID 解析为 uint64。
func ParseID(fieldName, s string) (uint64, error) {
	id, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid %s %q: %w", fieldName, s, err)
	}
	if id == 0 {
		return 0, fmt.Errorf("invalid %s: must be positive, got %q", fieldName, s)
	}
	return id, nil
}
