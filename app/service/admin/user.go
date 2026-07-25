package admin

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	"meta-api/app/model/admin"
	userModel "meta-api/app/model/user"
	"meta-api/common/cachekey"
	"meta-api/common/constants"
	"meta-api/common/idutil"
	"meta-api/common/types"
	"meta-api/common/utils"
)

// UserGetAboutMe 获取关于我
func (a *adminService) UserGetAboutMe(ctx context.Context) (*types.GetAboutMeResponse, error) {
	// 获取缓存
	response := &types.GetAboutMeResponse{}
	aboutMeKey := cachekey.AboutMeHash().String()
	if exist := a.redis.Exists(ctx, aboutMeKey).Val(); exist == 1 {
		fields := []string{"name", "job", "workLife", "address", "domainInfo", "blogContent", "websiteLocation", "statement", "email"}
		aboutMeInfo, err := a.redis.HMGet(ctx, aboutMeKey, fields...).Result()
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
		if err = a.redis.HSet(ctx, aboutMeKey, aboutMeMap).Err(); err != nil {
			a.logger.Error("failed to set aboutMeInfo to redis", zap.Error(err))
			return response, err
		}
	}
	return response, nil
}

func (a *adminService) AdminGetUserList(ctx context.Context,
	request *types.AdminGetUserListRequest) (*types.AdminGetUserListResponse, error) {

	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return nil, err
	}
	filter := userModel.AdminListFilter{
		Handle:            normalizeAdminUserHandle(request.Handle),
		DisplayName:       strings.TrimSpace(request.DisplayName),
		Provider:          strings.TrimSpace(request.Provider),
		CommentPermission: strings.TrimSpace(request.CommentPermission),
		Now:               time.Now().In(loc),
		Offset:            (request.Page - 1) * request.PageSize,
		Limit:             request.PageSize,
	}

	rows, total, err := a.userModel.ListUsers(ctx, filter)
	if err != nil {
		a.logger.Error("failed to list users", zap.Error(err))
		return nil, err
	}

	items := make([]types.AdminUserItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, toAdminUserItem(row, filter.Now))
	}
	return &types.AdminGetUserListResponse{
		Rows:  items,
		Total: int(total),
	}, nil
}

func (a *adminService) AdminUpdateUserCommentPermission(ctx context.Context,
	request *types.AdminUpdateUserCommentPermissionRequest) error {

	id, err := idutil.ParseID("userID", request.ID)
	if err != nil {
		return err
	}
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return err
	}
	now := time.Now().In(loc)
	reason := strings.TrimSpace(request.Reason)
	var disabledUntil *time.Time
	if request.Disabled {
		disabledUntil, err = parseAdminUserDisabledUntil(request.DisabledUntil, loc)
		if err != nil {
			return err
		}
	} else {
		reason = ""
	}
	return a.userModel.UpdateCommentPermission(ctx, id, request.Disabled, reason, disabledUntil, now)
}

func (a *adminService) AdminForceUserLogout(ctx context.Context, request *types.AdminForceUserLogoutRequest) error {
	id, err := idutil.ParseID("userID", request.ID)
	if err != nil {
		return err
	}
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return err
	}
	return a.userModel.IncrementSessionVersion(ctx, id, time.Now().In(loc))
}

func toAdminUserItem(row userModel.AdminListItem, now time.Time) types.AdminUserItem {
	commentDisabled := row.CommentDisabled && (row.CommentDisabledUntil == nil || row.CommentDisabledUntil.After(now))
	item := types.AdminUserItem{
		ID:                    strconv.FormatUint(row.ID, 10),
		Provider:              row.Provider,
		ProviderUserID:        row.ProviderUserID,
		DisplayName:           row.DisplayName,
		Handle:                row.Handle,
		AvatarURL:             row.AvatarURL,
		ProfileURL:            row.ProfileURL,
		Email:                 row.Email,
		CommentDisabled:       commentDisabled,
		CommentDisabledReason: row.CommentDisabledReason,
		SessionVersion:        row.SessionVersion,
		CommentCount:          row.CommentCount,
		CreateTime:            row.CreateTime.Format(constants.TimeLayoutToMinute),
		UpdateTime:            row.UpdateTime.Format(constants.TimeLayoutToMinute),
	}
	if !commentDisabled {
		item.CommentDisabledReason = ""
	}
	if row.CommentDisabledUntil != nil && commentDisabled {
		item.CommentDisabledUntil = row.CommentDisabledUntil.Format(constants.TimeLayoutToMinute)
	}
	if row.LastCommentTime != nil {
		item.LastCommentTime = row.LastCommentTime.Format(constants.TimeLayoutToMinute)
	}
	return item
}

func parseAdminUserDisabledUntil(value string, loc *time.Location) (*time.Time, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil, nil
	}
	parsed, err := time.ParseInLocation(constants.TimeLayoutToSecond, trimmed, loc)
	if err != nil {
		parsed, err = time.ParseInLocation(constants.TimeLayoutToMinute, trimmed, loc)
	}
	if err != nil {
		return nil, fmt.Errorf("invalid disabled until %q: %w", value, err)
	}
	return &parsed, nil
}

func normalizeAdminUserHandle(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	number, err := strconv.ParseUint(trimmed, 10, 64)
	if err != nil {
		return trimmed
	}
	if number > 0 && number < 100000 {
		return fmt.Sprintf("%05d", number)
	}
	return strconv.FormatUint(number, 10)
}
