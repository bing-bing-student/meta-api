package sms

import (
	"github.com/alibabacloud-go/darabonba-openapi/v2/client"
	dysmsapi20170525 "github.com/alibabacloud-go/dysmsapi-20170525/v3/client"
	"github.com/alibabacloud-go/tea/tea"

	"meta-api/common/utils"
)

const (
	envAliyunAccessKeyID     = "ALIYUN_ACCESS_KEY_ID"
	envAliyunAccessKeySecret = "ALIYUN_ACCESS_KEY_SECRET"
)

// CreateClient 创建客户端
func CreateClient() (result *dysmsapi20170525.Client, err error) {
	accessKeyID, err := utils.EnvOrFile(envAliyunAccessKeyID)
	if err != nil {
		return nil, err
	}
	accessKeySecret, err := utils.EnvOrFile(envAliyunAccessKeySecret)
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
