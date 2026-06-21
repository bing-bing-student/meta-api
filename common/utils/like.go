package utils

import "strings"

// EscapeLike 转义 LIKE 通配符，避免用户输入中的 \ % _ 改变匹配语义
func EscapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}
