// Package cachekey 集中管理项目所有 Redis 缓存键。
//
// 设计目标：
//  1. 所有 Key 通过类型化函数生成，避免在业务代码中出现裸字符串拼接。
//  2. Key 使用专门的 Key 类型而非 string，防止把任意字符串当作 Key 误传。
//  3. 同一类 Key 的命名规范、Score 公式集中维护，方便统一改造（含未来加 namespace、做 redis 集群迁移等）。
//
// 命名约定：{module}:{dimension}[:{id}][:{type}]
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
