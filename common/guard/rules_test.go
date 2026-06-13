package guard

import "testing"

// TestIsInternalReferer 覆盖双信号判定：
//   - sec-fetch-site=same-origin → true（首选信号）
//   - sec-fetch-site=cross-site / same-site → false（明确跨源，不再兜底）
//   - sec-fetch-site 缺失 + referer 是 http(s) → true（旧浏览器兜底）
//   - sec-fetch-site 缺失 + referer 空 / 非 http(s) → false
func TestIsInternalReferer(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		referer  string
		secFetch string
		want     bool
	}{
		{
			name:     "sec-fetch-site=same-origin",
			referer:  "",
			secFetch: "same-origin",
			want:     true,
		},
		{
			name:     "sec-fetch-site=cross-site 不应兜底 referer",
			referer:  "https://liubing.xyz/",
			secFetch: "cross-site",
			want:     false,
		},
		{
			name:     "sec-fetch-site=same-site 不应兜底 referer",
			referer:  "https://liubing.xyz/",
			secFetch: "same-site",
			want:     false,
		},
		{
			name:     "sec-fetch-site 缺失 + referer https → 兜底命中",
			referer:  "https://liubing.xyz/",
			secFetch: "",
			want:     true,
		},
		{
			name:     "sec-fetch-site 缺失 + referer http → 不兜底（线上 HSTS 强制 https，http 多为攻击）",
			referer:  "http://localhost:3000/",
			secFetch: "",
			want:     false,
		},
		{
			name:     "sec-fetch-site 缺失 + 攻击者从 http 域伪造 referer → 不兜底",
			referer:  "http://attacker.example/",
			secFetch: "",
			want:     false,
		},
		{
			name:     "sec-fetch-site 缺失 + referer 空 → false",
			referer:  "",
			secFetch: "",
			want:     false,
		},
		{
			name:     "sec-fetch-site 缺失 + referer 非 http(s) → false",
			referer:  "android-app://com.example/",
			secFetch: "",
			want:     false,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := isInternalReferer(tc.referer, tc.secFetch)
			if got != tc.want {
				t.Fatalf("isInternalReferer(%q, %q) = %v, want %v",
					tc.referer, tc.secFetch, got, tc.want)
			}
		})
	}
}

// TestCheckL2Score_SameOriginNoDoubleCount 验证"同源信号只加一次分"。
//
// 修复前：sec-fetch=same-origin 命中 navigate 加分 +5，
// 同时 isInternalReferer(_, "same-origin")=true 又加 +5，合计 +10（双倍注水）。
// 修复后：两路径取其一加分，合计 +5。
func TestCheckL2Score_SameOriginNoDoubleCount(t *testing.T) {
	t.Parallel()

	r := rules{}

	// 构造一份"中性"请求：所有扣分项都不命中、其它加分项都不命中，
	// 只看同源加分一项。
	req := &RiskRequest{
		Scene:     SceneViewLog,
		UserAgent: "X-Custom-Agent/1.0", // 不命中黑/白名单
		Lang:      "",                   // 关闭 lang 相关扣分/加分
		Screen:    "",
		PerfNav:   "",
		SecFetch: SecFetchHeaders{
			Mode:           "navigate",
			Site:           "same-origin",
			Dest:           "document",
			AcceptLanguage: "zh-CN",
		},
		Referer: "https://liubing.xyz/article-detail/abc",
	}

	score := r.checkL2Score(req)

	// 起始 80 + 同源加分 5 = 85（其它加分项均不命中）
	want := scoreL2Start + scoreSameOriginNav
	if score != want {
		t.Fatalf("score = %d, want %d (起始 %d + 同源 %d)",
			score, want, scoreL2Start, scoreSameOriginNav)
	}
}

// TestCheckL2Score_RefererFallback 验证"sec-fetch-site 缺失时 referer 兜底加分"。
func TestCheckL2Score_RefererFallback(t *testing.T) {
	t.Parallel()

	r := rules{}

	req := &RiskRequest{
		Scene:     SceneViewLog,
		UserAgent: "X-Custom-Agent/1.0",
		Lang:      "",
		Screen:    "",
		PerfNav:   "",
		SecFetch: SecFetchHeaders{
			// 旧浏览器：sec-fetch-* 全缺
			Mode:           "",
			Site:           "",
			Dest:           "",
			AcceptLanguage: "",
		},
		Referer: "https://liubing.xyz/",
	}

	score := r.checkL2Score(req)

	// sec-fetch 全缺扣 4 * 15 = 60；referer 兜底加 5。
	// 80 - 60 + 5 = 25
	want := scoreL2Start - 4*deductSecFetchMiss + scoreSameOriginNav
	if score != want {
		t.Fatalf("score = %d, want %d", score, want)
	}
}

// TestCheckL2Score_CrossSiteNoBonus 验证"sec-fetch-site=cross-site 时 referer 不再兜底加分"。
//
// 攻击场景：攻击者伪造 Referer 头但 sec-fetch-site=cross-site 暴露真实跨源；
// 修复后此路径不应享受同源加分，避免被注水。
func TestCheckL2Score_CrossSiteNoBonus(t *testing.T) {
	t.Parallel()

	r := rules{}

	req := &RiskRequest{
		Scene:     SceneViewLog,
		UserAgent: "X-Custom-Agent/1.0",
		Lang:      "",
		Screen:    "",
		PerfNav:   "",
		SecFetch: SecFetchHeaders{
			Mode:           "cors",
			Site:           "cross-site",
			Dest:           "empty",
			AcceptLanguage: "zh-CN",
		},
		Referer: "https://liubing.xyz/", // 攻击者伪造
	}

	score := r.checkL2Score(req)

	// 起始 80，所有扣分项都不命中，所有加分项都不命中（cross-site）：80
	want := scoreL2Start
	if score != want {
		t.Fatalf("score = %d, want %d (cross-site 不应享受同源加分)",
			score, want)
	}
}

// TestCheckL2Score_FastTS_NoDeduct 验证"client_ts 距 serverNow 极近不再扣分"。
//
// 真实环境（容器内网 / 同城 RTT）下 delta 通常仅 2~10ms，过去会被
// deductTSTooFast 一刀切扣 30 分。本用例确保即便 delta=2ms 也不扣分。
// 时间戳伪造检测由 engine 层 TSSkewToleranceMs (±60s) 统一保障。
func TestCheckL2Score_FastTS_NoDeduct(t *testing.T) {
	t.Parallel()

	r := rules{}

	req := &RiskRequest{
		Scene:     SceneViewLog,
		UserAgent: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 Chrome/149.0.0.0 Safari/537.36",
		Lang:      "zh-CN",
		Screen:    "1920x1080",
		PerfNav:   "navigate",
		SecFetch: SecFetchHeaders{
			Mode:           "cors",
			Site:           "same-origin",
			Dest:           "empty",
			AcceptLanguage: "zh-CN,zh;q=0.9",
		},
		Referer: "https://liubing.xyz/article-detail/123",
	}

	score := r.checkL2Score(req)

	// 起始 80 + same-origin 5 + lang 3 + known browser 3 + perfNav 5 = 96
	want := scoreL2Start + scoreSameOriginNav + scoreLangMatch +
		scoreKnownBrowser + scorePerfNavigate
	if score != want {
		t.Fatalf("score = %d, want %d (真实浏览器请求应保持高分，不应被 ts_too_fast 误扣)",
			score, want)
	}
}
