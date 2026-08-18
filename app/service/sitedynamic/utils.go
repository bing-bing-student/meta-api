package sitedynamic

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	siteDynamicModel "meta-api/app/model/sitedynamic"
	"meta-api/common/constants"
	"meta-api/common/types"
)

func parseSiteDynamicEventDate(value string) (time.Time, error) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return time.Time{}, fmt.Errorf("load location: %w", err)
	}
	parsed, err := time.ParseInLocation(constants.TimeLayoutToDay, strings.TrimSpace(value), loc)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid event date: %w", err)
	}
	return parsed, nil
}

func siteDynamicNow() (time.Time, error) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return time.Time{}, fmt.Errorf("load location: %w", err)
	}
	return time.Now().In(loc), nil
}

func toAdminSiteDynamicItem(row siteDynamicModel.SiteDynamic) types.AdminSiteDynamicItem {
	return types.AdminSiteDynamicItem{
		ID:         strconv.FormatUint(row.ID, 10),
		Content:    row.Content,
		Status:     row.Status,
		SortOrder:  row.SortOrder,
		EventDate:  row.EventTime.Format(constants.TimeLayoutToDay),
		CreateTime: row.CreateTime.Format(constants.TimeLayoutToMinute),
		UpdateTime: row.UpdateTime.Format(constants.TimeLayoutToMinute),
	}
}

func toUserSiteDynamicItem(row siteDynamicModel.SiteDynamic) types.UserSiteDynamicItem {
	return types.UserSiteDynamicItem{
		Content:   row.Content,
		EventDate: row.EventTime.Format(constants.TimeLayoutToDay),
	}
}
