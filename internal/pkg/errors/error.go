package errors

const (
	Success = 2000 // 成功

	BadRequest     = 4000 // 参数错误
	Unauthorized   = 4010 // 未授权（缺少Token）
	AuthFailed     = 4011 // 认证失败（错误的token）
	TokenExpired   = 4012 // Token过期
	Forbidden      = 4030 // 禁止访问（请求IP被封禁、被限流等原因）
	NotFound       = 4040 // 资源不存在（访问被删除的文章等）
	RequestTimeout = 4080 // 请求超时（网络连接失败等）

	InternalServerError = 5000 // 服务内部错误
)
