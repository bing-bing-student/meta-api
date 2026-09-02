package moderation

import (
	"sort"
	"strings"

	appconfig "meta-api/config"
)

// FeedbackCategories 从统一分类注册表生成管理员可选择的人工反馈分类。
// 分类不再在后端、审核规则和管理端分别维护，避免新增分类时出现三套列表不一致。
func FeedbackCategories(cfg appconfig.CommentModerationConfig) []string {
	values := make([]string, 0, len(cfg.Categories))
	for id, category := range cfg.Categories {
		id = strings.ToLower(strings.TrimSpace(id))
		if id != "" && category.FeedbackEnabled {
			values = append(values, id)
		}
	}
	sort.Strings(values)
	return values
}

// IsFeedbackCategory 判断 value 是否存在于 cfg 的统一分类注册表且允许用于人工反馈。
// 返回 true 表示管理员可提交该分类。
func IsFeedbackCategory(value string, cfg appconfig.CommentModerationConfig) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	category, ok := cfg.Categories[value]
	return ok && category.FeedbackEnabled
}
