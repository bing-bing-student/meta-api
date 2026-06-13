package guard

import (
	"encoding/binary"
	"math"
	"testing"
)

// TestParseSummary_RoundTrip 用同样的字节布局回写一个 summary 并验证解析结果。
func TestParseSummary_RoundTrip(t *testing.T) {
	want := BehaviorSummary{
		SampleCount:        60,
		DurationMs:         3000,
		MousePathLenPx:     1234,
		MouseSpeedMean:     1.5,
		MouseSpeedStd:      0.4,
		MouseJerkMean:      0.02,
		MouseMaxDy:         12.5,
		ClickCount:         2,
		ClickDwellMeanMs:   80,
		KeydownCount:       4,
		KeyFlightMeanMs:    150,
		WheelCount:         1,
		WheelDeltaSum:      -120,
		FocusChanges:       3,
		VisibilityHiddenMs: 200,
		Flags:              BehaviorFlagBlur,
	}

	buf := encodeSummary(&want)
	if len(buf) != SummaryBinaryLen {
		t.Fatalf("encoded length: got %d, want %d", len(buf), SummaryBinaryLen)
	}
	got, ok := parseSummary(buf)
	if !ok {
		t.Fatal("parseSummary returned ok=false")
	}
	if got.SampleCount != want.SampleCount ||
		got.DurationMs != want.DurationMs ||
		got.ClickCount != want.ClickCount ||
		got.WheelDeltaSum != want.WheelDeltaSum ||
		got.Flags != want.Flags {
		t.Fatalf("integer fields mismatch: got=%+v want=%+v", got, want)
	}
	if !approxF32(got.MouseSpeedMean, want.MouseSpeedMean) ||
		!approxF32(got.MouseJerkMean, want.MouseJerkMean) {
		t.Fatalf("float fields mismatch: got=%+v want=%+v", got, want)
	}
}

// TestBehaviorEvaluator 关键场景的得分检查。
func TestBehaviorEvaluator(t *testing.T) {
	t.Parallel()

	eval := behaviorEvaluator{}

	t.Run("nil summary penalized but not zero", func(t *testing.T) {
		score, _ := eval.Evaluate(nil)
		if score == 0 || score >= 100 {
			t.Fatalf("unexpected score for nil summary: %d", score)
		}
	})

	t.Run("dwell too short → DWELL_TOO_SHORT", func(t *testing.T) {
		// 停留 <3s（兜底打点路径常见）：硬门槛，0 分。
		// 即使有事件、有 Flag 也直接拒。
		s := &BehaviorSummary{
			DurationMs:  1500,
			SampleCount: 3,
			ClickCount:  1,
			Flags:       BehaviorFlagVisibilityHidden,
		}
		score, reason := eval.Evaluate(s)
		if score != 0 || reason != "DWELL_TOO_SHORT" {
			t.Fatalf("got score=%d reason=%s, want 0 / DWELL_TOO_SHORT", score, reason)
		}
	})

	t.Run("all zero long → ALL_ZERO_LONG", func(t *testing.T) {
		// 完全静止 ≥2.5s：应判 ALL_ZERO_LONG，30 分。
		// 注意必须 ≥ MinDwellMs=3000 才能跳过 DWELL_TOO_SHORT。
		s := &BehaviorSummary{DurationMs: 3500}
		score, reason := eval.Evaluate(s)
		if score != 30 || reason != "ALL_ZERO_LONG" {
			t.Fatalf("got score=%d reason=%s, want 30 / ALL_ZERO_LONG", score, reason)
		}
	})

	t.Run("early leave → EARLY_LEAVE_OK", func(t *testing.T) {
		// 用户至少看了 3s 后切走（visibilitychange→hidden 或 blur）：70 分。
		s := &BehaviorSummary{
			DurationMs: 3500,
			Flags:      BehaviorFlagVisibilityHidden,
		}
		score, reason := eval.Evaluate(s)
		if score != 70 || reason != "EARLY_LEAVE_OK" {
			t.Fatalf("got score=%d reason=%s, want 70 / EARLY_LEAVE_OK", score, reason)
		}
	})

	t.Run("weak samples → WEAK_SAMPLES", func(t *testing.T) {
		// 非空但 SampleCount<5：60 分。
		s := &BehaviorSummary{
			DurationMs:  3500,
			SampleCount: 0,
			ClickCount:  1,
		}
		score, reason := eval.Evaluate(s)
		if score != 60 || reason != "WEAK_SAMPLES" {
			t.Fatalf("got score=%d reason=%s, want 60 / WEAK_SAMPLES", score, reason)
		}
	})

	t.Run("no interaction (≥3s 0 事件 0 Flag) → ALL_ZERO_LONG", func(t *testing.T) {
		// v1.1 引入 DWELL_TOO_SHORT 硬门槛后，DurationMs<3000 被前置拦截，
		// 走到 NO_INTERACTION 分支需要 DurationMs≥3000，但此时同时也满足
		// ALL_ZERO_LONG 条件（≥2500 + 无事件 + 无 Flag）；后者优先。
		// NO_INTERACTION 分支保留作为防御性兜底（理论上能命中的场景：
		// DurationMs∈[3000,2500) 不存在，所以实际上当前规则下不会走到）。
		s := &BehaviorSummary{DurationMs: 3000}
		score, reason := eval.Evaluate(s)
		if score != 30 || reason != "ALL_ZERO_LONG" {
			t.Fatalf("got score=%d reason=%s, want 30 / ALL_ZERO_LONG", score, reason)
		}
	})

	t.Run("constant speed signals bot", func(t *testing.T) {
		s := &BehaviorSummary{
			SampleCount:    60,
			DurationMs:     3000,
			MouseSpeedMean: 1.0,
			MouseSpeedStd:  0.0, // 完美匀速
			MouseJerkMean:  0.0,
		}
		score, _ := eval.Evaluate(s)
		// 至少要被扣到 30 分以下（40 + 30 = 70 扣减）
		if score >= 50 {
			t.Fatalf("expected bot-like score < 50, got %d", score)
		}
	})

	t.Run("focus hint boosts score", func(t *testing.T) {
		s := &BehaviorSummary{
			SampleCount:   20,
			DurationMs:    3000,
			MouseSpeedStd: 0.5,
			MouseJerkMean: 0.1,
			Flags:         BehaviorFlagBlur,
		}
		score, _ := eval.Evaluate(s)
		if score < 90 {
			t.Fatalf("human-like sample should keep high score, got %d", score)
		}
	})
}

// ---- helpers ----

func encodeSummary(s *BehaviorSummary) []byte {
	out := make([]byte, SummaryBinaryLen)
	be := binary.BigEndian
	be.PutUint16(out[0:2], s.SampleCount)
	be.PutUint32(out[2:6], s.DurationMs)
	be.PutUint32(out[6:10], s.MousePathLenPx)
	be.PutUint32(out[10:14], math.Float32bits(s.MouseSpeedMean))
	be.PutUint32(out[14:18], math.Float32bits(s.MouseSpeedStd))
	be.PutUint32(out[18:22], math.Float32bits(s.MouseJerkMean))
	be.PutUint32(out[22:26], math.Float32bits(s.MouseMaxDy))
	be.PutUint16(out[26:28], s.ClickCount)
	be.PutUint32(out[28:32], s.ClickDwellMeanMs)
	be.PutUint16(out[32:34], s.KeydownCount)
	be.PutUint32(out[34:38], s.KeyFlightMeanMs)
	be.PutUint16(out[38:40], s.WheelCount)
	be.PutUint32(out[40:44], uint32(s.WheelDeltaSum))
	be.PutUint16(out[44:46], s.FocusChanges)
	be.PutUint32(out[46:50], s.VisibilityHiddenMs)
	be.PutUint16(out[50:52], s.Flags)
	return out
}

func approxF32(a, b float32) bool {
	const eps = 1e-5
	d := a - b
	if d < 0 {
		d = -d
	}
	return d < eps
}
