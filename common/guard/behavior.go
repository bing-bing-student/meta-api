package guard

import (
	"encoding/binary"
	"fmt"
	"math"
)

// BehaviorSummary 与前端 crypto-wasm/src/behavior.rs 中 `BehaviorSummary` 严格对齐。
//
// 二进制布局（Big-Endian，紧凑排布，pad 到 64B 收尾）：
//
//	offset  size  field
//	0       2     SampleCount       u16
//	2       4     DurationMs        u32
//	6       4     MousePathLenPx    u32
//	10      4     MouseSpeedMean    f32
//	14      4     MouseSpeedStd     f32
//	18      4     MouseJerkMean     f32
//	22      4     MouseMaxDy        f32
//	26      2     ClickCount        u16
//	28      4     ClickDwellMeanMs  u32
//	32      2     KeydownCount      u16
//	34      4     KeyFlightMeanMs   u32
//	38      2     WheelCount        u16
//	40      4     WheelDeltaSum     i32
//	44      2     FocusChanges      u16
//	46      4     VisibilityHiddenMs u32
//	50      2     Flags             u16
//	52      12    padding (zero)
type BehaviorSummary struct {
	SampleCount        uint16
	DurationMs         uint32
	MousePathLenPx     uint32
	MouseSpeedMean     float32
	MouseSpeedStd      float32
	MouseJerkMean      float32
	MouseMaxDy         float32
	ClickCount         uint16
	ClickDwellMeanMs   uint32
	KeydownCount       uint16
	KeyFlightMeanMs    uint32
	WheelCount         uint16
	WheelDeltaSum      int32
	FocusChanges       uint16
	VisibilityHiddenMs uint32
	Flags              uint16
}

// Behavior summary flag 位定义（与前端 behavior.rs 对齐）。
const (
	BehaviorFlagVisibilityHidden uint16 = 1 << 0
	BehaviorFlagBlur             uint16 = 1 << 1
)

// SummaryBinaryLen 与前端 SUMMARY_LEN 一致（64B）。
const SummaryBinaryLen = 64

// parseSummary 把 64B 二进制解码为 BehaviorSummary。
//
// 入参短于 SummaryBinaryLen 时返回 (nil, false)；
// 入参长度更大允许通过（取前 SummaryBinaryLen 字节，剩余忽略）。
func parseSummary(buf []byte) (*BehaviorSummary, bool) {
	if len(buf) < SummaryBinaryLen {
		return nil, false
	}
	be := binary.BigEndian
	s := &BehaviorSummary{
		SampleCount:        be.Uint16(buf[0:2]),
		DurationMs:         be.Uint32(buf[2:6]),
		MousePathLenPx:     be.Uint32(buf[6:10]),
		MouseSpeedMean:     math.Float32frombits(be.Uint32(buf[10:14])),
		MouseSpeedStd:      math.Float32frombits(be.Uint32(buf[14:18])),
		MouseJerkMean:      math.Float32frombits(be.Uint32(buf[18:22])),
		MouseMaxDy:         math.Float32frombits(be.Uint32(buf[22:26])),
		ClickCount:         be.Uint16(buf[26:28]),
		ClickDwellMeanMs:   be.Uint32(buf[28:32]),
		KeydownCount:       be.Uint16(buf[32:34]),
		KeyFlightMeanMs:    be.Uint32(buf[34:38]),
		WheelCount:         be.Uint16(buf[38:40]),
		WheelDeltaSum:      int32(be.Uint32(buf[40:44])),
		FocusChanges:       be.Uint16(buf[44:46]),
		VisibilityHiddenMs: be.Uint32(buf[46:50]),
		Flags:              be.Uint16(buf[50:52]),
	}
	return s, true
}

// behaviorEvaluator 行为评分器。规则集与 spec §6.4 一致，初版保守取阈值。
//
// 之所以用结构体而不是包级函数，是为了未来支持"按场景动态调阈值" / "灰度用 A/B 评分函数"。
type behaviorEvaluator struct{}

// Evaluate 基于前端 wasm 聚合好的 BehaviorSummary 打分。
//
// 返回 (score, reason)：score ∈ [0,100]；reason 是命中的具体规则名（用于审计）。
//
// view-log 场景下调用方不会再以"score < BehaviorScoreThreshold"硬拒，
// 而是把本函数返回的 score 与 L2 软合议（见 engine.combineForViewLog）。
// share-create 等强交互场景仍按 BehaviorScoreThreshold 硬阈值判拒。
//
// 评分起始判定分级（替代 v1 的"<5 样本一刀切 25 分"）：
//   - DWELL_TOO_SHORT: 0 分（产品硬门槛：停留 <3s 无效浏览，不参与软合议）
//   - ALL_ZERO_LONG : 30 分（headless 默认空运行）
//   - EARLY_LEAVE_OK: 70 分（用户 3s 内切走，真人特征）
//   - WEAK_SAMPLES  : 60 分（采样不足但非空，弱信号不直接判 bot）
//   - 其它          : 走 100 起步 + 规则集扣减
//
// 设计取舍：v1.x 所有规则均基于 wasm 已压缩的 summary 字段，不再消费
// FieldBehaviorRaw 原始事件流（性价比低 + 链路体积大）。若未来 v1.2
// 需要做"轨迹自相关 / 间隔分布"这类二次分析，再单独引入 raw 入参，
// 同步规划 BUILD_HASH 滚动。
func (b *behaviorEvaluator) Evaluate(s *BehaviorSummary) (int, string) {
	// 不存在 summary 视为可疑（极端 bot 场景：完全不动 + 直接 fetch）。
	if s == nil {
		return 30, "NO_SUMMARY"
	}

	// 起始分级 0（最高优先级）：停留时长不足 MinDwellMs 一律视为无效浏览。
	// 产品口径：< 3s 即使是真人也算误触/秒退，不计 view_count。
	// 返回 0 分 + DWELL_TOO_SHORT 原因码；engine 在 SceneViewLog 分支内
	// 短路走独立拒因（VIEWLOG_DWELL_TOO_SHORT），不进入软合议。
	if s.DurationMs < MinDwellMs {
		return 0, "DWELL_TOO_SHORT"
	}

	totalEvents := uint32(s.SampleCount) + uint32(s.ClickCount) +
		uint32(s.KeydownCount) + uint32(s.WheelCount)

	// 起始分级 1：完全静止 ≥2.5s 且无任何 Flag → 强 bot 信号
	// （headless 默认；prerender 已在 L1 拦掉，能走到这里说明攻击者刻意伪造）。
	if totalEvents == 0 && s.Flags == 0 && s.DurationMs >= 2500 {
		return 30, "ALL_ZERO_LONG"
	}

	// 起始分级 2：用户秒退 / 切走（visibilitychange→hidden 或 blur）。
	// 注意 v1.1 引入 DWELL_TOO_SHORT 后，能走到这里说明 DurationMs ≥ 3000，
	// 即用户至少看了 3 秒才切走，给中等偏高分数避免误伤。
	if s.Flags&(BehaviorFlagVisibilityHidden|BehaviorFlagBlur) != 0 &&
		s.SampleCount < 5 {
		return 70, "EARLY_LEAVE_OK"
	}

	// 起始分级 3：弱采样（非全空但样本不足）。
	// 例如 PC 用户进文章只点了一下/滚了一下就停下阅读。
	// 在 v1.1 view-log 软合议下，60 分 + L2 高分（≥85）合议后 ≈75，仍可通过。
	if s.SampleCount < 5 && totalEvents > 0 {
		return 60, "WEAK_SAMPLES"
	}

	// 起始分级 4：移动端 / PC 静读且无任何交互信号（既无 hidden/blur，也无事件）。
	// 此情况在 v1 被一刀切判为 25 分；v1.1 给 50 分，配合 L2 软合议
	// 让"真实浏览器 + 高 L2 分"的用户能通过，"伪造浏览器 + 低 L2 分"被拒。
	if totalEvents == 0 {
		return 50, "NO_INTERACTION"
	}

	score := 100
	reasons := make([]string, 0, 4)

	// 规则 1：速度近常数（mouseSpeedStd 极低 + 样本量足够）→ 强 bot 信号
	if s.MouseSpeedStd < 0.05 && s.SampleCount > 20 {
		score -= 40
		reasons = append(reasons, "SPEED_CONST")
	}

	// 规则 2：抖动指数过低（人类抖动均值 > 0.001 px/ms²）
	if s.MouseJerkMean < 0.001 && s.SampleCount > 20 {
		score -= 30
		reasons = append(reasons, "JERK_LOW")
	}

	// 规则 3：极端短的点击 dwell（< 30ms 几乎不可能是真人）
	if s.ClickCount > 0 && s.ClickDwellMeanMs < 30 {
		score -= 25
		reasons = append(reasons, "CLICK_FAST")
	}

	// 规则 4：键间飞行时间过短且键事件较多 → 自动化 typing
	if s.KeydownCount > 5 && s.KeyFlightMeanMs > 0 && s.KeyFlightMeanMs < 30 {
		score -= 30
		reasons = append(reasons, "KEY_FAST")
	}

	// 规则 5：曾切到背景 / 失焦 → 真人特征，上调
	if s.Flags&(BehaviorFlagVisibilityHidden|BehaviorFlagBlur) != 0 {
		score += 10
		reasons = append(reasons, "HUMAN_FOCUS_HINT")
	}

	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}

	if len(reasons) == 0 {
		return score, fmt.Sprintf("score=%d", score)
	}
	return score, fmt.Sprintf("score=%d|%s", score, joinReasons(reasons))
}

// joinReasons 不引入 strings 包的临时拼接。
func joinReasons(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	total := len(parts) - 1
	for _, p := range parts {
		total += len(p)
	}
	out := make([]byte, 0, total)
	for i, p := range parts {
		if i > 0 {
			out = append(out, ',')
		}
		out = append(out, p...)
	}
	return string(out)
}
