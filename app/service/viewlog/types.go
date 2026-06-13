package viewlog

// Outcome service 层判定结果。handler 据此映射 HTTP 响应。
type Outcome struct {
	HTTPStatus int    // 204 / 404 / 500
	Code       int    // 业务码：0 表示无 body（204 场景）
	Message    string // 业务消息：仅 Code != 0 时使用
}

const (
	codeNotFound      = 4040
	codeInternalError = 5000
)
