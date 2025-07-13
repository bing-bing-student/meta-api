package utils

import (
	"github.com/bytedance/sonic"
)

// JSONToString 将结构体序列化为字符串
func JSONToString(info interface{}) (string, error) {
	jsonBytes, err := sonic.Marshal(info)
	if err != nil {
		return "", err
	}
	return string(jsonBytes), nil
}

// StringToJSON 将字符串反序列化为结构体
func StringToJSON(jsonString string, info interface{}) error {
	return sonic.UnmarshalString(jsonString, info)
}
