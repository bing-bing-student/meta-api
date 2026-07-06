// Package guard 是风控守卫方案 v1 的通用风控引擎。
//
// 适用场景：
//   - 文章详情"浏览量 +1"
//   - JSON 工具"分享创建"
//
// 设计目标：
//  1. 端到端二进制信封协议，与前端 crypto-wasm（guard-core）byte-for-byte 对齐。
//     协议规格见：portal-web/docs/anti-bot-guard-v1-spec.md §3
//  2. 业务侧只需一句 `engine.Evaluate(ctx, &RiskRequest{...})`，
//     即可拿到统一的 Outcome，根据 Decision 决定继续业务或拒绝。
//  3. 与现有 keyManager / cacheKey / 审计日志风格保持一致，
//     不引入新的依赖（仅使用标准库 + go-redis + zap）。
//
// 文件分布：
//
//	types.go    —— Scene / Decision / RiskRequest / Outcome / 错误码
//	engine.go   —— Engine 接口 + impl 主流程 + 构造
//	envelope.go —— 二进制信封解码 + TLV 解析
//	decrypt.go  —— AES-256-GCM + RSA-2048-OAEP + HMAC-SHA256
//	behavior.go —— BehaviorSummary 解析 + 评分
//	rules.go    —— L1 黑名单 + L2 评分
//	store.go    —— Redis nonce / rate / dedup 抽象
//	builds.go   —— BUILD_HASH 白名单（支持灰度多版本共存）
//	audit.go    —— zap 审计日志（与 viewlog/audit.go 风格一致）
package guard

import "time"

// Scene 场景常量，与前端 crypto-wasm 中的 scene 字节一一对应。
type Scene uint8

// 场景定义。新增场景需同时同步前端 crypto-wasm 的常量与 spec §3.1。
const (
	SceneViewLog     Scene = 0x10
	SceneShareCreate Scene = 0x20
)

// String 给 Scene 提供可读名称（仅用于日志/审计，不参与协议）。
func (s Scene) String() string {
	switch s {
	case SceneViewLog:
		return "view-log"
	case SceneShareCreate:
		return "share-create"
	default:
		return "unknown"
	}
}

// Decision Engine 评估结果的最终决策。
type Decision int

// Decision 枚举。HTTPStatus / 是否计数等映射由调用方负责。
const (
	// DecisionAccept 通过所有风控，业务可以执行。
	DecisionAccept Decision = iota
	// DecisionSilent 静默拒绝（建议返回 204，不暴露差异）。
	DecisionSilent
	// DecisionBadRequest 请求参数 / 协议错误。
	DecisionBadRequest
	// DecisionRateLimited 频控触发。
	DecisionRateLimited
	// DecisionNotFound 目标资源不存在（例如 articleId 已删除）。
	DecisionNotFound
	// DecisionInternal 引擎自身或下游异常。
	DecisionInternal
)

// SecFetchHeaders 浏览器侧 Sec-Fetch-* 与相关 header 的快照。
//
// 字段为空字符串表示请求未携带（不强制要求；缺失只在 L2 软扣分）。
type SecFetchHeaders struct {
	Mode string
	Site string
	Dest string
	// AcceptLanguage 不属于 Sec-Fetch 但同源参与扣分判定，放在一起方便传递。
	AcceptLanguage string
}

// RiskRequest 风控引擎评估的所有入参。
//
// RawBody 是经过 io.LimitReader 截断后的二进制信封原始字节，
// Engine 内部完成解码 + 解密 + 校验。
type RiskRequest struct {
	Scene     Scene
	TargetID  string
	RawBody   []byte
	ClientIP  string
	UserAgent string

	SecFetch SecFetchHeaders

	// Referer 仅作为 L2 同源弱信号使用，不参与硬拒绝判定。
	Referer string

	// 软指标（前端用 client_meta TLV 上报，JSON 容器化字段）。
	// 兼容老接口：viewlog 旧版 RSA token 链路下线前可继续填入这些字段；
	// 新链路下这些字段会从 envelope.PayloadFields[FieldClientMeta] 中解析。
	TZ      string
	Lang    string
	Screen  string
	PerfNav string
}

// Outcome Engine 评估的输出。
type Outcome struct {
	Decision Decision
	// Reason 审计/日志用的拒因码。e.g. "ENV_DECODE_FAIL" / "L1_UA" / "BEH_BOT_LIKE"
	Reason string
	// Score L2 + 行为联合评分（0~100），仅供审计参考。
	Score int

	// 解出的字段，供 Handler 业务使用（如 fingerprint 用于继续频控）。
	Fingerprint string
	// PayloadFields 全量 TLV 解出的字段，供高级用法读取；常规用法不需要。
	PayloadFields map[uint8][]byte
}

// 引擎内部使用的固定常量。
const (
	// MaxBodyBytes Engine 接受的最大信封字节数。Handler 应使用 io.LimitReader 截断到这个值再传入。
	MaxBodyBytes = 16 * 1024

	// TSSkewToleranceMs 客户端时钟容忍窗口（±2 分钟）。
	TSSkewToleranceMs int64 = 120_000

	// NonceTTL Redis nonce SETNX 的过期时间，与 dedup 对齐。
	NonceTTL = 60 * time.Second

	// DedupTTL (fp, targetID) 主去重窗口。
	DedupTTL = 60 * time.Second

	// L2ScoreThreshold L2 总评分低于此值直接静默拒。
	L2ScoreThreshold = 60
	// BehaviorScoreThreshold 行为得分低于此值静默拒（仅 share-create 等强交互场景）。
	BehaviorScoreThreshold = 50
	// ViewLogFinalScoreThreshold view-log 场景 L2+L4 软合议后的最终拒绝阈值。
	ViewLogFinalScoreThreshold = 60

	// MinDwellMs view-log 场景"最小有效停留时长"。低于此值视为无效浏览，
	// 直接判 DWELL_TOO_SHORT，不参与软合议（产品口径硬门槛）。
	// 与前端 VIEW_LOG_DELAY_MS=3000 对齐：定时器路径下页面 3s 后才发，
	// 此分支只可能在 pagehide / visibilitychange 兜底打点路径下命中。
	MinDwellMs uint32 = 3000
)

// 拒因码集合（reason 字符串）。Handler / 监控告警按此 grep。
const (
	ReasonBodyEmpty        = "ENV_BODY_EMPTY"
	ReasonBodyTooLarge     = "ENV_BODY_TOO_LARGE"
	ReasonEnvDecodeFail    = "ENV_DECODE_FAIL"
	ReasonMagicMismatch    = "ENV_MAGIC_MISMATCH"
	ReasonVersionMismatch  = "ENV_VERSION_MISMATCH"
	ReasonSceneMismatch    = "ENV_SCENE_MISMATCH"
	ReasonRSAFail          = "RSA_FAIL"
	ReasonAESFail          = "AES_FAIL"
	ReasonHMACFail         = "HMAC_FAIL"
	ReasonTLVFail          = "TLV_FAIL"
	ReasonTargetMismatch   = "TARGET_MISMATCH"
	ReasonTSSkew           = "TS_SKEW"
	ReasonNonceReplay      = "NONCE_REPLAY"
	ReasonNonceFail        = "NONCE_FAIL"
	ReasonBuildHashUnknown = "BUILD_HASH_UNKNOWN"
	ReasonFingerprintBad   = "FINGERPRINT_BAD"

	ReasonL1UA        = "L1_UA"
	ReasonL1Prerender = "L1_PRERENDER"
	ReasonL1Header    = "L1_HEADER"
	ReasonL2Score     = "L2_SCORE"
	ReasonL3Dedup     = "L3_DEDUP"
	ReasonL3RateIP    = "L3_RATE_IP"
	ReasonL3RateFP    = "L3_RATE_FP"
	ReasonL4Behavior  = "L4_BEHAVIOR"
	// ReasonViewLogCombined view-log 场景 L2+L4 合议低于阈值。
	// 后缀附加 L4 reason，便于审计定位（例如 "VIEWLOG_COMBINED:ALL_ZERO_LONG"）。
	ReasonViewLogCombined = "VIEWLOG_COMBINED"
	// ReasonViewLogDwellTooShort view-log 场景停留时长不足。
	// 走独立拒因码（不挂在 VIEWLOG_COMBINED 后），方便监控/审计区分
	// "无效浏览（停留过短）" vs "可疑请求（合议不达标）"。
	ReasonViewLogDwellTooShort = "VIEWLOG_DWELL_TOO_SHORT"
	ReasonInternalError        = "INTERNAL_ERROR"
)

// AcceptedReason 表示判定通过时使用的占位 reason，便于审计日志统一字段。
const AcceptedReason = "ACCEPTED"
