package bootstrap

import (
	"github.com/robfig/cron/v3"
)

// InitCron 初始化定时任务
func InitCron() *cron.Cron {
	return cron.New()
}
