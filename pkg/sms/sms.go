package sms

import (
	"encoding/json"
	"errors"
	"strings"

	dysmsapi20170525 "github.com/alibabacloud-go/dysmsapi-20170525/v3/client"
	util "github.com/alibabacloud-go/tea-utils/v2/service"
	"github.com/alibabacloud-go/tea/tea"
	"github.com/bytedance/sonic"
)

// SendMessage 发送短信验证码
func SendMessage(phone string) (code string, err error) {
	client, err := CreateClient()
	if err != nil {
		return code, err
	}

	code, err = GenerateRandomDigits(6)
	if err != nil {
		return code, err
	}

	identifyingCodeString, err := sonic.Marshal(SMS{Code: code})
	if err != nil {
		return code, err
	}
	sendSmsRequest := &dysmsapi20170525.SendSmsRequest{
		SignName:      tea.String("BingBingBlog"),
		TemplateCode:  tea.String("SMS_296325152"),
		PhoneNumbers:  tea.String(phone),
		TemplateParam: tea.String(string(identifyingCodeString)),
	}
	tryErr := func() (e error) {
		defer func() {
			if err = tea.Recover(recover()); err != nil {
				e = err
			}
		}()
		if _, err = client.SendSmsWithOptions(sendSmsRequest, &util.RuntimeOptions{}); err != nil {
			return err
		}
		return err
	}()

	if tryErr != nil {
		var e = &tea.SDKError{}
		var t *tea.SDKError
		if errors.As(tryErr, &t) {
			e = t
		}
		// 诊断地址
		var data interface{}
		d := json.NewDecoder(strings.NewReader(tea.StringValue(e.Data)))
		if err = d.Decode(&data); err != nil {
			return code, err
		}
		if _, err = util.AssertAsString(e.Message); err != nil {
			return code, err
		}
	}

	// TODO: 待重构，将code存入redis当中
	//if err = global.RedisSentinel.Set(global.Context, "code", code, time.Minute).Err(); err != nil {
	//	return err
	//}
	return code, err
}
