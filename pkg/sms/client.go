package sms

import (
	"os"

	"github.com/alibabacloud-go/darabonba-openapi/v2/client"
	dysmsapi20170525 "github.com/alibabacloud-go/dysmsapi-20170525/v3/client"
	"github.com/alibabacloud-go/tea/tea"
)

// CreateClient  创建客户端
func CreateClient() (result *dysmsapi20170525.Client, err error) {
	// 初始化配置
	configInfo := &client.Config{
		AccessKeyId:     tea.String(os.Getenv("ALIYUN_ACCESS_KEY_ID")),
		AccessKeySecret: tea.String(os.Getenv("ALIYUN_ACCESS_KEY_SECRET")),
	}
	configInfo.Endpoint = tea.String("dysmsapi.aliyuncs.com")

	// 创建客户端
	result = &dysmsapi20170525.Client{}
	result, err = dysmsapi20170525.NewClient(configInfo)
	return result, err
}
