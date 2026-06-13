package cachekey

import "strings"

// Key 缓存键的类型化包装。
// 调用 Redis API 时用 string(k) 或 k.String() 转换。
type Key string

// String 实现 fmt.Stringer，便于日志打印
func (k Key) String() string { return string(k) }

// build 以 ":" 连接 parts，统一构造分层 Key
func build(parts ...string) Key {
	return Key(strings.Join(parts, ":"))
}
