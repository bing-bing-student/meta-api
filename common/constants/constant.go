package constants

const (
	TimeLayoutToDay    = "2006-01-02"
	TimeLayoutToMinute = "2006-01-02 15:04"
	TimeLayoutToSecond = "2006-01-02 15:04:05"

	StartTime = "2023-01-01 00:00:01" // 固定启动时间，保证生成 ID 唯一性

	Spec = "0 3 * * *" // 定时任务表达式

	MaxFileSize = int64(64 << 10) // MD文件大小限制为64KB
)
