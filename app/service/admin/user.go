package admin

import (
	"context"
	"strings"

	"go.uber.org/zap"

	"meta-api/app/model/admin"
	"meta-api/common/types"
	"meta-api/common/utils"
)

// UserGetAboutMe 获取关于我
func (a *adminService) UserGetAboutMe(ctx context.Context) (*types.GetAboutMeResponse, error) {
	// 获取缓存
	response := &types.GetAboutMeResponse{}
	if exist := a.redis.Exists(ctx, "aboutMeInfo:Hash").Val(); exist == 1 {
		fields := []string{"name", "job", "workLife", "address", "domainInfo", "blogContent", "websiteLocation", "statement", "email"}
		aboutMeInfo, err := a.redis.HMGet(ctx, "aboutMeInfo:Hash", fields...).Result()
		if err != nil {
			a.logger.Error("failed to get aboutMeInfo from redis", zap.Error(err))
			return response, err
		}
		response.Name = aboutMeInfo[0].(string)
		response.Job = aboutMeInfo[1].(string)
		response.WorkLife = aboutMeInfo[2].(string)
		response.Address = aboutMeInfo[3].(string)
		response.DomainInfo = aboutMeInfo[4].(string)
		response.BlogContent = aboutMeInfo[5].(string)
		response.WebsiteLocation = aboutMeInfo[6].(string)
		response.Statement = aboutMeInfo[7].(string)
		emailStr, ok := aboutMeInfo[8].(string)
		if !ok {
			a.logger.Error("failed to get admin info", zap.Error(err))
			return response, err
		}
		response.Email = strings.Split(emailStr, ",")
	} else {
		// 获取管理员信息
		adminInfo, err := a.model.GetAdminInfo(ctx)
		if err != nil {
			a.logger.Error("failed to get admin info", zap.Error(err))
			return response, err
		}
		aboutMeInfo := admin.AboutMeInfo{}
		if err = utils.JsonStringToStruct(adminInfo.AboutMeInfo, &aboutMeInfo); err != nil {
			a.logger.Error("failed to unmarshal aboutMeInfo", zap.Error(err))
			return response, err
		}
		response.Name = aboutMeInfo.Name
		response.Job = aboutMeInfo.Job
		response.WorkLife = aboutMeInfo.WorkLife
		response.Address = aboutMeInfo.Address

		webSiteInfo := admin.WebSiteInfo{}
		if err = utils.JsonStringToStruct(adminInfo.WebSiteInfo, &webSiteInfo); err != nil {
			a.logger.Error("failed to unmarshal webSiteInfo", zap.Error(err))
			return response, err
		}
		response.DomainInfo = webSiteInfo.DomainInfo
		response.BlogContent = webSiteInfo.BlogContent
		response.WebsiteLocation = webSiteInfo.WebsiteLocation
		response.Statement = webSiteInfo.Statement

		contactMeInfo := admin.ContactMeInfo{}
		if err = utils.JsonStringToStruct(adminInfo.ContactMeInfo, &contactMeInfo); err != nil {
			a.logger.Error("failed to unmarshal contactMeInfo", zap.Error(err))
			return response, err
		}
		response.Email = contactMeInfo.Email

		// 写入缓存
		aboutMeMap := map[string]interface{}{
			"name":            response.Name,
			"job":             response.Job,
			"workLife":        response.WorkLife,
			"address":         response.Address,
			"domainInfo":      response.DomainInfo,
			"blogContent":     response.BlogContent,
			"websiteLocation": response.WebsiteLocation,
			"statement":       response.Statement,
			"email":           strings.Join(response.Email, ","),
		}
		if err = a.redis.HSet(ctx, "aboutMeInfo:Hash", aboutMeMap).Err(); err != nil {
			a.logger.Error("failed to set aboutMeInfo to redis", zap.Error(err))
			return response, err
		}
	}
	return response, nil
}
