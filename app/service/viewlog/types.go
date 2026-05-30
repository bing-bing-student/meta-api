package viewlog

// PostViewLogRequest 接口入参聚合：URL 路径里的 articleId + 请求体里的 token / 软指标。
type PostViewLogRequest struct {
	// URL 路径参数 :id（与 token 中的 articleId 必须一致）
	ArticleID string `json:"-"`

	// 请求体字段
	Token   string `json:"token" binding:"required"`
	Referer string `json:"referer"`
	TZ      string `json:"tz"`
	Screen  string `json:"screen"`
	Lang    string `json:"lang"`
	PerfNav string `json:"perfNav"`

	// 后端自行采集，由 handler 注入（前端送了也忽略）
	IP             string `json:"-"`
	UserAgent      string `json:"-"`
	AcceptLanguage string `json:"-"`
	SecFetchMode   string `json:"-"`
	SecFetchSite   string `json:"-"`
	SecFetchDest   string `json:"-"`
}

// TokenPayload RSA 解密 + JSON 解析后的明文结构。
type TokenPayload struct {
	FingerprintID string `json:"fingerprintId"`
	ArticleID     string `json:"articleId"`
	TS            int64  `json:"ts"` // 毫秒时间戳
	Nonce         string `json:"nonce"`
}

// Outcome service 层判定结果。handler 据此映射 HTTP 响应。
type Outcome struct {
	HTTPStatus int    // 204 / 400 / 404 / 429 / 500
	Code       int    // 业务码：0 表示无 body（204 场景）
	Message    string // 业务消息：仅 Code != 0 时使用
}

const (
	codeBadRequest     = 4000
	codeNotFound       = 4040
	codeRateLimited    = 4290
	codeInternalError  = 5000
)

const (
	decisionAccepted = "accepted"
	decisionRejected = "rejected"

	reasonTokenInvalid          = "TOKEN_INVALID"
	reasonTokenTSSkew           = "TOKEN_TS_SKEW"
	reasonTokenReplay           = "TOKEN_REPLAY"
	reasonTokenArticleMismatch  = "TOKEN_ARTICLE_MISMATCH"
	reasonArticleNotFound       = "ARTICLE_NOT_FOUND"
	reasonL1UA                  = "L1_UA"
	reasonL1Prerender           = "L1_PRERENDER"
	reasonL1Header              = "L1_HEADER"
	reasonL2Referer             = "L2_REFERER"
	reasonL2Score               = "L2_SCORE"
	reasonL3Dedup               = "L3_DEDUP"
	reasonL3RateIP              = "L3_RATE_IP"
	reasonL3RateFP              = "L3_RATE_FP"
	reasonInternalError         = "INTERNAL_ERROR"
)

// L2 评分常量（起点 100，结束后 < 60 拒）。
const (
	scoreStart        = 100
	scoreThreshold    = 60
	deductLangMismatch = 30
	deductScreenSmall  = 20
	deductTSTooFast    = 30
	deductSecFetchMiss = 15 // 单个 sec-fetch 头缺失（在 L1 判定为不直接拒后用作软扣分）
)

// 时间窗口与最小客户端间隔。
const (
	tsSkewToleranceMs   = 120_000 // ±2 分钟
	tsMinClientDelayMs  = 200     // 真人 onMounted → 1.5s setTimeout 的最少间隔
	screenMinWidth      = 320
	screenMinHeight     = 480
	uaTruncateBytes     = 512
	refererTruncateBytes = 512
)
