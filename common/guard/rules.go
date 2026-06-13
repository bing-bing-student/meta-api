package guard

import "strings"

// uaBlacklist UA 子串黑名单（小写匹配）。
//
// 复用 viewlog 服务原有的列表内容；放在 guard 包内以便 share / 其它场景共享。
var uaBlacklist = []string{
	"bot", "spider", "crawl", "slurp",
	"googlebot", "baiduspider", "bingbot", "yandexbot", "duckduckbot",
	"headless", "phantomjs",
	"curl", "wget", "python-requests", "go-http-client",
}

// allowedSecFetchModes 与 viewlog/risk.go 保持一致：
//
//	包含 "no-cors" 是因为 sendBeacon + Blob 会被浏览器自动标记为 simple request；
//	其它枚举值是浏览器在常规导航 / fetch 下使用的合法 Mode。
var allowedSecFetchModes = map[string]struct{}{
	"cors":        {},
	"same-origin": {},
	"navigate":    {},
	"no-cors":     {},
}

// L2 评分常量。
//
// 起始分 80 给"正向信号"留出加分空间；扣分项与历史一致。
// 调整背景：v1 起始 100 全靠扣分判定，对"被动浏览（PC 静读 / 移动端不滑动）"
// 这类弱采集场景下限不足。v1.1 引入 sec-fetch / referer / lang / UA / perfNav
// 五项正向信号，让真实浏览器用户保持高分。
const (
	scoreL2Start       = 80
	deductLangMismatch = 30
	deductScreenSmall  = 20
	deductSecFetchMiss = 15
	screenMinWidth     = 320
	screenMinHeight    = 480

	// 正向信号加分项（命中即加，不命中则不影响）。
	scoreSameOriginNav = 5  // sec-fetch-mode=navigate && site=same-origin 或 referer 兜底
	scoreLangMatch     = 3  // navigator.language 与 Accept-Language 主语种一致
	scoreKnownBrowser  = 3  // UA 命中已知主流浏览器关键字
	scorePerfNavigate  = 5  // PerformanceNavigationTiming.type == "navigate"

	scoreL2Max = 100
)

// scoreStart 兼容别名。
// 历史 reject 路径会传 scoreStart 作为 fallback，含义是"未真正打分时的占位高分"，
// 与 L2 起始分耦合。保持别名以减少调用点扩散修改。
const scoreStart = scoreL2Start

// knownBrowserKeywords UA 加分白名单（小写匹配）。
//
// 与 uaBlacklist 互不冲突：黑名单优先，命中黑名单走 L1_UA 直接拒，
// 不会进入 L2 加分判定。
var knownBrowserKeywords = []string{
	"chrome/", "safari/", "firefox/", "edg/",
	"micromessenger", "mqqbrowser", "ucbrowser",
}

// rules L1/L2 规则集合。
type rules struct{}

// checkL1 硬规则黑名单。命中返回 (true, reason)，未命中返回 (false, "")。
func (rules) checkL1(req *RiskRequest) (bool, string) {
	if matchUABlacklist(req.UserAgent) {
		return true, ReasonL1UA
	}

	if req.PerfNav == "prerender" {
		return true, ReasonL1Prerender
	}

	if req.SecFetch.Mode != "" {
		if _, ok := allowedSecFetchModes[req.SecFetch.Mode]; !ok {
			return true, ReasonL1Header
		}
	}

	return false, ""
}

// checkL2Referer 站内 _payload.json 来源直接拒（爬虫预取常见）。
//
// 仅 view-log 场景使用。share-create 由前端按钮触发，不带可信 referer。
func (rules) checkL2Referer(req *RiskRequest) (bool, string) {
	if req.Scene == SceneViewLog && strings.Contains(req.Referer, "_payload.json") {
		return true, ReasonL2Referer
	}
	return false, ""
}

// checkL2Score 软扣分项 + 正向信号加分项。
//
// 评分流程：起始 80 → 扣分项（语言失配 / 屏幕过小 / sec-fetch 缺失）→
// 加分项（同源跳转 / 内站 referer / lang 一致 / 已知 UA / perfNav=navigate）→
// 上限 100、下限 0 截取。
//
// 时间戳防伪由 engine 层统一负责（见 TSSkewToleranceMs 全局校验：
// |serverNow - clientTS| > 60s 直接判 TS_SKEW 拒绝）。本层不再做
// "client_ts 距 serverNow 太近就扣分"——该规则方向与时钟伪造攻击相反
// （真实浏览器在容器内网 / 同城 RTT 通常 <10ms 反而都被误伤），
// 同时与"client_ts 太远才可疑"的 TS_SKEW 形成语义矛盾。
func (rules) checkL2Score(req *RiskRequest) int {
	score := scoreL2Start

	// ---- 扣分项 ----
	if req.Lang != "" && req.SecFetch.AcceptLanguage != "" &&
		!langConsistent(req.Lang, req.SecFetch.AcceptLanguage) {
		score -= deductLangMismatch
	}

	if w, h, ok := parseScreen(req.Screen); ok {
		if w < screenMinWidth || h < screenMinHeight {
			score -= deductScreenSmall
		}
	}

	if req.SecFetch.Mode == "" {
		score -= deductSecFetchMiss
	}
	if req.SecFetch.Site == "" {
		score -= deductSecFetchMiss
	}
	if req.SecFetch.Dest == "" {
		score -= deductSecFetchMiss
	}
	if req.SecFetch.AcceptLanguage == "" {
		score -= deductSecFetchMiss
	}

	// ---- 加分项（正向信号）----
	// 同源信号：以下两种来源任一命中即加分（避免双倍注水）。
	//   1. sec-fetch-mode=navigate && site=same-origin（现代浏览器首选，难伪造）
	//   2. 退化到 referer 兜底（旧浏览器 / 部分 webview 不带 sec-fetch-* 头）
	// 设计动机：原版本拆成两个独立加分项 A/B，A 命中时 B 必然也命中（旧实现里
	// isInternalReferer 只看 sec-fetch-site），导致同一信号被算两次 +10 分；
	// 改为"取其一加分"后，单一信号一次 +5，referer 兜底覆盖旧浏览器场景。
	if req.SecFetch.Mode == "navigate" && req.SecFetch.Site == "same-origin" {
		score += scoreSameOriginNav
	} else if isInternalReferer(req.Referer, req.SecFetch.Site) {
		score += scoreSameOriginNav
	}

	// 浏览器 navigator.language 主语种 ⊆ Accept-Language。
	if req.Lang != "" && req.SecFetch.AcceptLanguage != "" &&
		langConsistent(req.Lang, req.SecFetch.AcceptLanguage) {
		score += scoreLangMatch
	}

	// UA 命中主流浏览器关键字。注意 L1 黑名单已优先拦截恶意 UA，
	// 这里只为给真实浏览器加分。
	if isKnownRealBrowser(req.UserAgent) {
		score += scoreKnownBrowser
	}

	// PerformanceNavigationTiming.type=navigate（非 prerender / preload / back_forward）。
	if req.PerfNav == "navigate" {
		score += scorePerfNavigate
	}

	if score < 0 {
		score = 0
	}
	if score > scoreL2Max {
		score = scoreL2Max
	}
	return score
}

// matchUABlacklist 大小写不敏感子串匹配，空 UA 直接判中。
func matchUABlacklist(ua string) bool {
	if ua == "" {
		return true
	}
	lower := strings.ToLower(ua)
	for _, kw := range uaBlacklist {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// langConsistent Lang 主语言段（zh-CN → zh）出现在 Accept-Language 子串里即一致。
func langConsistent(lang, acceptLang string) bool {
	primary := strings.ToLower(lang)
	if i := strings.Index(primary, "-"); i > 0 {
		primary = primary[:i]
	}
	return strings.Contains(strings.ToLower(acceptLang), primary)
}

// parseScreen 解析 "1920x1080" → (1920, 1080, true)。
func parseScreen(s string) (int, int, bool) {
	idx := strings.Index(s, "x")
	if idx <= 0 || idx == len(s)-1 {
		return 0, 0, false
	}
	w, ok1 := atoiNonNeg(s[:idx])
	h, ok2 := atoiNonNeg(s[idx+1:])
	if !ok1 || !ok2 {
		return 0, 0, false
	}
	return w, h, true
}

// atoiNonNeg 仅接受非负十进制字符串。
func atoiNonNeg(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	n := 0
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return 0, false
		}
		n = n*10 + int(ch-'0')
	}
	return n, true
}

// isInternalReferer 判断 referer 是否来自站内。
//
// 双信号判定：
//  1. Sec-Fetch-Site == "same-origin"（首选，浏览器自动填，攻击者难伪造）
//  2. sec-fetch 系列缺失时退化到 referer 兜底（旧浏览器 / 部分 webview）：
//     referer 非空且为 https:// 形式视为弱站内来源信号。
//
// 注意：referer 兜底是"弱信号"——攻击者可通过自定义 Header 伪造 Referer，
// 因此本函数返回值只用于 L2 加分，不参与拒绝判定。
//
// 当 secFetchSite 已携带（无论是否 same-origin）时，referer 不再参与判定，
// 让上层调用方走"sec-fetch-site=same-origin"独立加分路径，避免与 referer 兜底重叠。
//
// 仅接受 https://：线上站点强制 HSTS，真实用户的 referer 必为 https；
// http:// 出现时多半来自攻击者从非 HSTS 域伪造请求，不应享受站内加分。
// 本地开发环境（localhost http）虽然命中此路径会少 5 分，但 score 80 → 75
// 仍远过阈值 60，对调试无实质影响。
func isInternalReferer(referer, secFetchSite string) bool {
	if secFetchSite == "same-origin" {
		return true
	}
	// sec-fetch-site 已携带但非 same-origin：明确的跨源信号，不应再靠 referer 兜底
	if secFetchSite != "" {
		return false
	}
	// sec-fetch-site 缺失：referer 弱兜底，仅接受 https://
	if referer == "" {
		return false
	}
	return strings.HasPrefix(referer, "https://")
}

// isKnownRealBrowser UA 命中已知主流浏览器关键字。
//
// 不参与拒绝判定，仅用于 L2 加分，给真实浏览器在弱采集场景留余量。
// L1 黑名单（headless / curl / bot / spider 等）优先生效。
func isKnownRealBrowser(ua string) bool {
	if ua == "" {
		return false
	}
	lower := strings.ToLower(ua)
	for _, kw := range knownBrowserKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}
