package sms

import (
	"github.com/alibabacloud-go/darabonba-openapi/v2/client"
	dysmsapi20170525 "github.com/alibabacloud-go/dysmsapi-20170525/v3/client"
	"github.com/alibabacloud-go/tea/tea"

	"meta-api/common/env"
	"meta-api/common/utils"
)

// CreateClient 创建客户端
func CreateClient() (result *dysmsapi20170525.Client, err error) {
	accessKeyID, err := utils.EnvOrFile(env.AliyunAccessKeyID)
	if err != nil {
		return nil, err
	}
	accessKeySecret, err := utils.EnvOrFile(env.AliyunAccessKeySecret)
	if err != nil {
		return nil, err
	}
	config := &client.Config{
		AccessKeyId:     tea.String(accessKeyID),
		AccessKeySecret: tea.String(accessKeySecret),
		Endpoint:        tea.String("dysmsapi.aliyuncs.com"),
	}

	// 创建客户端
	result, err = dysmsapi20170525.NewClient(config)
	return result, err
}
