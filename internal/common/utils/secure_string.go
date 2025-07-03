package utils

type SecureString struct {
	value []byte
}

// NewSecureString 安全内存存储
func NewSecureString(s string) *SecureString {
	b := []byte(s)
	return &SecureString{value: b}
}

func (ss *SecureString) Get() string {
	return string(ss.value)
}

func (ss *SecureString) Clear() {
	for i := range ss.value {
		ss.value[i] = 0
	}
	ss.value = nil
}
