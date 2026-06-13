package guard

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"time"

	"go.uber.org/zap"

	"meta-api/pkg/keymanager"
)

// Engine 风控守卫评估引擎。
//
// 业务侧只需调用 Evaluate 一次，根据 Outcome.Decision 决定继续业务或拒绝。
// 错误返回 != nil 表示引擎内部异常（应映射为 500），业务级别的拒绝走 Outcome。
type Engine interface {
	Evaluate(ctx context.Context, req *RiskRequest) (*Outcome, error)
}

// EngineConfig Engine 构造参数。除 KeyManager / Store / Logger 必填外其余可选。
type EngineConfig struct {
	KeyManager *keymanager.Manager
	Store      Store
	Logger     *zap.Logger
	// BuildHashes 当前接受的 build_hash 白名单。可为空（开发态），engine 会按 SkipHMACWhenEmpty 行为处理。
	BuildHashes *BuildHashRegistry
	// SkipHMACWhenEmpty 当 BuildHashes 为空时是否跳过 HMAC 校验。
	// 仅本地开发 / 灰度初期建议开启；生产应保持 false（默认）。
	SkipHMACWhenEmpty bool

	// Now 时间钩子，方便单元测试。零值时使用 time.Now。
	Now func() time.Time
}

// engine 的具体实现。
type engine struct {
	km          *keymanager.Manager
	store       Store
	logger      *zap.Logger
	buildHashes *BuildHashRegistry
	skipHMAC    bool

	rules    rules
	behavior behaviorEvaluator

	now func() time.Time
}

// NewEngine 构造 Engine。
//
// KeyManager / Store / Logger 必填，缺失返回 error；BuildHashes 缺省构造空 registry。
func NewEngine(cfg EngineConfig) (Engine, error) {
	if cfg.KeyManager == nil {
		return nil, errors.New("guard: KeyManager required")
	}
	if cfg.Store == nil {
		return nil, errors.New("guard: Store required")
	}
	if cfg.Logger == nil {
		return nil, errors.New("guard: Logger required")
	}
	if cfg.BuildHashes == nil {
		cfg.BuildHashes = NewBuildHashRegistry()
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &engine{
		km:          cfg.KeyManager,
		store:       cfg.Store,
		logger:      cfg.Logger,
		buildHashes: cfg.BuildHashes,
		skipHMAC:    cfg.SkipHMACWhenEmpty,
		now:         cfg.Now,
	}, nil
}

// Evaluate NewEngine 完成上方 Engine 接口的实现注入。所有依赖必须显式声明，
// 不提供"便捷构造"——便捷构造容易写出风险默认值（如 SkipHMACWhenEmpty=true +
// 空 BuildHashRegistry，等同于风控旁路），DI 层应直接使用 EngineConfig 装配。
//
// Evaluate 主流程。所有阶段都按"先解码/校验 → 后业务规则"的顺序串联。
//
// 任一阶段判定为拒：填充 audit + Outcome 后立即返回，不再继续后续阶段。
func (e *engine) Evaluate(ctx context.Context, req *RiskRequest) (*Outcome, error) {
	if req == nil {
		return nil, errors.New("guard: nil request")
	}
	serverNowMs := e.now().UnixMilli()
	out := &Outcome{Score: scoreStart}
	// 在解出 TLV 之前 clientTS 还未知，统一传 0；解出后改写为真实值用于 reject 审计。
	var clientTSMs int64

	// ---- 1. 信封解码（仅格式 + 长度）----
	ev, err := decodeEnvelope(req.RawBody)
	if err != nil {
		return e.reject(req, out, DecisionBadRequest, reasonForDecodeError(err), 0, serverNowMs), nil
	}

	// ---- 2. 校验 scene 与 URL 一致 ----
	if ev.Scene != uint8(req.Scene) {
		return e.reject(req, out, DecisionBadRequest, ReasonSceneMismatch, 0, serverNowMs), nil
	}

	// ---- 3. RSA-OAEP 解出 (aes_key || iv || nonce_seed) ----
	keyBlob, err := e.km.DecryptOAEP(ev.RsaCiphertext)
	if err != nil {
		if errors.Is(err, keymanager.ErrNotReady) {
			e.logger.Warn("guard rsa decrypt: keyManager not ready", zap.String("scene", req.Scene.String()))
		}
		return e.reject(req, out, DecisionBadRequest, ReasonRSAFail, 0, serverNowMs), nil
	}
	km, ok := parseKeyMaterial(keyBlob)
	if !ok {
		return e.reject(req, out, DecisionBadRequest, ReasonRSAFail, 0, serverNowMs), nil
	}

	// ---- 4. AES-256-GCM 解出 plaintext ----
	plaintext, err := aesGcmOpen(km.AESKey[:], ev.IV, ev.Tag, ev.Ciphertext)
	if err != nil {
		return e.reject(req, out, DecisionBadRequest, ReasonAESFail, 0, serverNowMs), nil
	}

	// ---- 5. HMAC 校验信封完整性 + build_hash ----
	if !e.skipHMACVerify() {
		candidates := e.buildHashes.Snapshot()
		ok, _ := verifyHMAC(ev.PrefixForHmac, ev.HMAC, km.AESKey[:], candidates)
		if !ok {
			return e.reject(req, out, DecisionBadRequest, ReasonHMACFail, 0, serverNowMs), nil
		}
	}

	// ---- 6. 解析 TLV ----
	fields, err := parseTLV(plaintext)
	if err != nil {
		return e.reject(req, out, DecisionBadRequest, ReasonTLVFail, 0, serverNowMs), nil
	}
	out.PayloadFields = fields

	// 6.1 fingerprint
	fpBytes, ok := fields[FieldFingerprintID]
	if !ok || len(fpBytes) != fieldFingerprintIDLen {
		return e.reject(req, out, DecisionBadRequest, ReasonFingerprintBad, 0, serverNowMs), nil
	}
	fpHex := hex.EncodeToString(fpBytes)
	out.Fingerprint = fpHex

	// 6.2 target id
	targetIDBytes, ok := fields[FieldTargetID]
	if !ok || string(targetIDBytes) != req.TargetID {
		return e.reject(req, out, DecisionBadRequest, ReasonTargetMismatch, 0, serverNowMs), nil
	}

	// 6.3 timestamp
	tsBytes, ok := fields[FieldTimestampMs]
	if !ok || len(tsBytes) != fieldTimestampMsLen {
		return e.reject(req, out, DecisionBadRequest, ReasonTLVFail, 0, serverNowMs), nil
	}
	clientTSMs = int64(binary.BigEndian.Uint64(tsBytes))
	if absInt64(serverNowMs-clientTSMs) > TSSkewToleranceMs {
		return e.rejectWithTS(req, out, DecisionBadRequest, ReasonTSSkew, 0, clientTSMs, serverNowMs), nil
	}

	// 6.4 nonce SETNX
	nonceBytes, ok := fields[FieldNonce]
	if !ok || len(nonceBytes) != fieldNonceLen {
		return e.rejectWithTS(req, out, DecisionBadRequest, ReasonTLVFail, 0, clientTSMs, serverNowMs), nil
	}
	noncePut, err := e.store.NonceTrySet(ctx, req.Scene, nonceBytes, NonceTTL)
	if err != nil {
		// Redis 抖动属于"看不出 token 是否合法"，按保守策略拒绝。
		e.logger.Warn("guard nonce setNX failed", zap.Error(err))
		return e.rejectWithTS(req, out, DecisionBadRequest, ReasonNonceFail, 0, clientTSMs, serverNowMs), nil
	}
	if !noncePut {
		return e.rejectWithTS(req, out, DecisionBadRequest, ReasonNonceReplay, 0, clientTSMs, serverNowMs), nil
	}

	// ---- 7. L1 黑名单 ----
	if hit, reason := e.rules.checkL1(req); hit {
		return e.rejectWithTS(req, out, DecisionSilent, reason, scoreStart, clientTSMs, serverNowMs), nil
	}

	// ---- 8. L2 referer 直接拒（仅 view-log） ----
	if hit, reason := e.rules.checkL2Referer(req); hit {
		return e.rejectWithTS(req, out, DecisionSilent, reason, scoreStart, clientTSMs, serverNowMs), nil
	}

	// ---- 9. L2 软评分 ----
	score := e.rules.checkL2Score(req)
	out.Score = score
	if score < L2ScoreThreshold {
		return e.rejectWithTS(req, out, DecisionSilent, ReasonL2Score, score, clientTSMs, serverNowMs), nil
	}

	// ---- 10. L3 频控（仅计数维度，每次请求都要累加） ----
	if reason, decision, ok := e.checkRate(ctx, req, fpHex); !ok {
		return e.rejectWithTS(req, out, decision, reason, score, clientTSMs, serverNowMs), nil
	}

	// ---- 11. L4 行为评分 ----
	// 与 L3 dedup 同：列表 / 统计这类占位 targetID 场景豁免行为评分：
	// 这些查询通常是用户点"刷新"瞬时触发，recorder 几乎没有采集窗口
	// （SampleCount < 5），行为分必然偏低，但这并非 bot 信号。
	// 风险代偿：nonce 防重放 + L3 频控（IP/fp）已能限制脚本刷列表的速率。
	summaryBytes := fields[FieldBehaviorSummary]
	summary, _ := parseSummary(summaryBytes)
	behaviorScore, behaviorReason := e.behavior.Evaluate(summary)

	if !isPlaceholderTargetID(req.TargetID) {
		switch req.Scene {
		case SceneViewLog:
			// 硬门槛：停留时长不足直接拒，独立拒因码（不参与软合议）。
			// 产品口径：<3s 浏览视为无效，即使 L2 满分也不+1。
			// 与 ReasonViewLogCombined 区分：前者属于"无效浏览"（多为
			// 真实用户秒退），后者属于"可疑请求"（多为伪造/headless）。
			if behaviorReason == "DWELL_TOO_SHORT" {
				return e.rejectWithTS(req, out, DecisionSilent,
					ReasonViewLogDwellTooShort,
					behaviorScore, clientTSMs, serverNowMs), nil
			}
			// view-log 是"被动浏览"场景，行为分作为软信号与 L2 加权合议；
			// 不再单独对 behaviorScore < BehaviorScoreThreshold 硬拒。
			// 决策依据：finalScore = L2*0.7 + L4*0.3，低于 ViewLogFinalScoreThreshold 拒。
			//
			// 设计动机：v1 中"3s 内 SampleCount<5"会一刀切 25 分硬拒，
			// 误伤了"PC 静读 / 移动端不滑动 / 微信打开秒退"等真人场景。
			// v1.1 通过分级评分 + 多维证据合议解决。
			finalScore := combineForViewLog(score, behaviorScore)
			out.Score = finalScore
			if finalScore < ViewLogFinalScoreThreshold {
				return e.rejectWithTS(req, out, DecisionSilent,
					ReasonViewLogCombined+":"+behaviorReason,
					finalScore, clientTSMs, serverNowMs), nil
			}
		default:
			// share-create 等强交互场景：必有 click / 表单输入，
			// 行为分硬阈值是合理的——低分基本只可能是 bot 或脚本。
			combined := combineScore(score, behaviorScore)
			out.Score = combined
			if behaviorScore < BehaviorScoreThreshold {
				return e.rejectWithTS(req, out, DecisionSilent,
					ReasonL4Behavior+":"+behaviorReason,
					combined, clientTSMs, serverNowMs), nil
			}
		}
	} else {
		// 占位 targetID 场景：仍写入 combined score 供审计参考。
		out.Score = combineScore(score, behaviorScore)
	}

	// ---- 12. L3 主去重（必须放在所有可能拒绝的判定之后） ----
	// dedup 的语义是"这个 (fp, target) 已经被成功计过一次"——只有走到
	// 即将 Accept 的请求才应该占坑，否则会出现以下错误场景：
	//   - 用户秒退（<3s）触发 visibility 兜底打点 → 后端 L4 判 DWELL_TOO_SHORT 拒
	//   - 但若 dedup 在 L4 之前就写入，60s 内用户回来正常浏览第二次会被 L3_DEDUP 拦截
	//   - 表现：用户连续两次进同一篇文章，第二次浏览量不+1
	// 修复方式：把 DedupTrySet 后置到 L4 之后，让被 L4 拒的请求不再占 dedup 坑位。
	//
	// targetId 全 0 的占位符场景仍按既有约定豁免 dedup（见 isPlaceholderTargetID 注释）。
	if !isPlaceholderTargetID(req.TargetID) {
		dedupOK, err := e.store.DedupTrySet(ctx, req.Scene, fpHex, req.TargetID, DedupTTL)
		if err != nil {
			// Redis 抖动按"未去重"放行；nonce 已经覆盖 1 分钟内的同请求重放
			e.logger.Warn("guard dedup setNX failed", zap.Error(err))
		} else if !dedupOK {
			return e.rejectWithTS(req, out, DecisionSilent, ReasonL3Dedup, out.Score, clientTSMs, serverNowMs), nil
		}
	}

	// ---- 13. 通过 ----
	out.Decision = DecisionAccept
	out.Reason = AcceptedReason
	e.audit(&auditEntry{
		Scene:       req.Scene,
		TargetID:    req.TargetID,
		Fingerprint: fpHex,
		IP:          req.ClientIP,
		UserAgent:   req.UserAgent,
		Decision:    DecisionAccept,
		Reason:      AcceptedReason,
		Score:       out.Score,
		ClientTSMs:  clientTSMs,
		ServerTSMs:  serverNowMs,
	})
	return out, nil
}

// 频控阈值矩阵：(scene, dimension) → (window, threshold)。
//
// 阈值与 viewlog 现有配置一致；share 暂用更紧的 fp/分钟（防止 1 个浏览器无限刷分享）。
func (e *engine) checkRate(ctx context.Context, req *RiskRequest, fpHex string) (string, Decision, bool) {
	prefix := guardKeyPrefix + ":" + req.Scene.String() + ":rate"

	// IP / 分钟
	ipMinKey := prefix + ":ip:" + req.ClientIP + ":1m"
	if exceeded, err := e.store.IncrCheckRate(ctx, ipMinKey, time.Minute, ipPerMinute(req.Scene)); err != nil {
		e.logger.Warn("guard rate ip:1m incr failed", zap.Error(err))
	} else if exceeded {
		return ReasonL3RateIP, DecisionRateLimited, false
	}

	// IP / 小时
	ipHourKey := prefix + ":ip:" + req.ClientIP + ":1h"
	if exceeded, err := e.store.IncrCheckRate(ctx, ipHourKey, time.Hour, ipPerHour(req.Scene)); err != nil {
		e.logger.Warn("guard rate ip:1h incr failed", zap.Error(err))
	} else if exceeded {
		return ReasonL3RateIP, DecisionRateLimited, false
	}

	// fingerprint / 分钟
	fpMinKey := prefix + ":fp:" + fpHex + ":1m"
	if exceeded, err := e.store.IncrCheckRate(ctx, fpMinKey, time.Minute, fpPerMinute(req.Scene)); err != nil {
		e.logger.Warn("guard rate fp:1m incr failed", zap.Error(err))
	} else if exceeded {
		return ReasonL3RateFP, DecisionRateLimited, false
	}

	return "", DecisionAccept, true
}

// 各场景的阈值集中在此，方便调整。
//
// view-log 与历史版本对齐（30 / 300 / 10）；
// share-create 适度放宽（20 / 120 / 10）：
//   - 用户在调试 JSON 时常会改一改再分享，单分钟 5 次很容易撞限
//   - dedup（同内容 5 分钟拦截）+ nonce + L1/L2 已经能挡机器刷分享，
//     这里把"真人合理操作"留出来更要紧
func ipPerMinute(s Scene) int64 {
	if s == SceneShareCreate {
		return 20
	}
	return 30
}

func ipPerHour(s Scene) int64 {
	if s == SceneShareCreate {
		return 120
	}
	return 300
}

func fpPerMinute(s Scene) int64 {
	if s == SceneShareCreate {
		return 10
	}
	return 10
}

// reject 填充 Outcome 并写一条审计日志。
//
// clientTSMs 在 TLV 解析成功后传入真实值；之前阶段 reject 传 0 即可（信封都未解码或 TS 未拿到）。
func (e *engine) reject(req *RiskRequest, out *Outcome, decision Decision, reason string, score int, serverNowMs int64) *Outcome {
	return e.rejectWithTS(req, out, decision, reason, score, 0, serverNowMs)
}

// rejectWithTS 是 reject 的扩展版，允许把已解码的 clientTSMs 一并写进审计。
func (e *engine) rejectWithTS(req *RiskRequest, out *Outcome, decision Decision, reason string, score int, clientTSMs, serverNowMs int64) *Outcome {
	out.Decision = decision
	out.Reason = reason
	if score > 0 {
		out.Score = score
	}
	e.audit(&auditEntry{
		Scene:       req.Scene,
		TargetID:    req.TargetID,
		Fingerprint: out.Fingerprint,
		IP:          req.ClientIP,
		UserAgent:   req.UserAgent,
		Decision:    decision,
		Reason:      reason,
		Score:       out.Score,
		ClientTSMs:  clientTSMs,
		ServerTSMs:  serverNowMs,
	})
	return out
}

// skipHMACVerify 是否跳过 HMAC 校验（开发期 BuildHashes 为空时允许）。
func (e *engine) skipHMACVerify() bool {
	return e.skipHMAC && e.buildHashes.Empty()
}

// combineScore L2 + 行为评分加权（0.6/0.4），保留整数。
//
// 用于 share-create 等"行为分硬阈值仍生效"的场景，仅作审计展示，
// 不影响判拒（判拒由 behaviorScore < BehaviorScoreThreshold 单独控制）。
func combineScore(l2, behavior int) int {
	combined := (l2*60 + behavior*40) / 100
	if combined < 0 {
		combined = 0
	}
	if combined > 100 {
		combined = 100
	}
	return combined
}

// combineForViewLog view-log 专用 L2+L4 加权（0.7/0.3）。
//
// 权重选取依据：view-log 场景行为分置信度天然低（3s 短窗口 + 被动浏览
// 不强制交互），所以让 L2（sec-fetch / lang / referer / UA / nav-timing
// 这类难伪造的浏览器画像）占主导 70%，行为分作为次要信号占 30%。
//
// 该值与 ViewLogFinalScoreThreshold(=60) 比较：
//   - L2=85, L4=60 → final=78  ✓
//   - L2=80, L4=50 → final=71  ✓ 真实浏览器静读
//   - L2=80, L4=30 → final=65  ✓ 弱信号但 L2 稳健
//   - L2=50, L4=30 → final=44  ✗ headless 伪造，sec-fetch 缺失被扣
//   - L2=50, L4=70 → final=56  ✗ headless 伪造行为却忘伪造浏览器画像
func combineForViewLog(l2, behavior int) int {
	combined := (l2*70 + behavior*30) / 100
	if combined < 0 {
		combined = 0
	}
	if combined > 100 {
		combined = 100
	}
	return combined
}

// absInt64 计算 int64 的绝对值（避免 math.Abs 转 float 精度损失）。
func absInt64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

// isPlaceholderTargetID 判断 targetId 是否为"无具体资源"占位符（32 字符全 0 的 hex）。
//
// 调用方约定：列表 / 统计这类无具体业务对象可绑的查询场景，
// 客户端在 sign 时使用 "00...0" (32 个 '0') 作为 targetId。
// engine 据此豁免 (fp, targetId) 去重，避免高频刷新被 L3_DEDUP 拦截。
func isPlaceholderTargetID(s string) bool {
	if len(s) != 32 {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] != '0' {
			return false
		}
	}
	return true
}
