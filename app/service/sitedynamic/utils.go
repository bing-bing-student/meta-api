package sitedynamic

import (
	"fmt"
	"strconv"
	"time"

	siteDynamicModel "meta-api/app/model/sitedynamic"
	"meta-api/common/constants"
	"meta-api/common/types"
)

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
		CreateTime: row.CreateTime.Format(constants.TimeLayoutToMinute),
		UpdateTime: row.UpdateTime.Format(constants.TimeLayoutToMinute),
	}
}

func toUserSiteDynamicItem(row siteDynamicModel.SiteDynamic) types.UserSiteDynamicItem {
	return types.UserSiteDynamicItem{
		Content: row.Content,
	}
}
