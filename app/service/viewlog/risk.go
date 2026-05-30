package viewlog

import (
	"context"
	"strings"

	"go.uber.org/zap"
)

// L1 / L2 / L3 风控判定。本文件不做任何 +1 决策，仅返回 *rejectOutcome；
// 上层 PostViewLog 负责审计日志与计数。
// 包含 "no-cors" 是因为前端通过 navigator.sendBeacon + Blob(text/plain) 发送时，
// 浏览器会自动把请求标记为 simple request，Sec-Fetch-Mode 取值固定为 "no-cors"，
// 前端无法手动覆盖。把它列入白名单避免误伤合法打点。
var allowedSecFetchModes = map[string]struct{}{
	"cors":        {},
	"same-origin": {},
	"navigate":    {},
	"no-cors":     {},
}

// checkL1 第 1 层：硬规则黑名单。
//
// 命中任一即静默 204，并填充对应 reason 用于审计日志。
//
// 说明：浏览器专属 Header（Sec-Fetch-Mode / Sec-Fetch-Site / Accept-Language）
// 缺失不再在 L1 直接拒，因为 Edge / Brave / Firefox 严格模式的跟踪防护会
// 剥离这些请求头，会误伤真实用户。改为在 L2 中按缺失数量软扣分。
func (s *viewLogService) checkL1(_ context.Context, req *PostViewLogRequest) *rejectOutcome {
	// UA 爬虫黑名单（子串匹配，case-insensitive）
	if s.matchUABlacklist(req.UserAgent) {
		return silentReject(reasonL1UA)
	}

	// 浏览器预渲染：用户实际未看
	if req.PerfNav == "prerender" {
		return silentReject(reasonL1Prerender)
	}

	// Sec-Fetch-Mode 异常（不在白名单内、且非空）
	// 空值可能是非现代浏览器或反代/隐私插件剥离，由 L2 软扣分覆盖
	if req.SecFetchMode != "" {
		if _, ok := allowedSecFetchModes[req.SecFetchMode]; !ok {
			return silentReject(reasonL1Header)
		}
	}

	return nil
}

// matchUABlacklist 大小写不敏感子串匹配 UA 黑名单。
// 名单为包级 var uaBlacklist（service.go），硬编码维护。
func (s *viewLogService) matchUABlacklist(ua string) bool {
	if ua == "" {
		// 完全无 UA 也直接拒：合法浏览器一定会发 UA
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

// checkL2 第 2 层：可信度评分 + referer 直接拒。
//
// 返回 (score, nil) 表示通过；(score, reject) 表示已判定拒绝。
func (s *viewLogService) checkL2(req *PostViewLogRequest, payload *TokenPayload, serverNowMs int64) (int, *rejectOutcome) {
	// 站内 _payload.json 来源 → 直接拒（爬虫/恶意预取常见）
	if strings.Contains(req.Referer, "_payload.json") {
		return scoreStart, silentReject(reasonL2Referer)
	}

	score := scoreStart

	// lang 与 Accept-Language 不一致（前端声明 zh-CN 但请求头不含 zh 任意变体）
	if req.Lang != "" && req.AcceptLanguage != "" && !langConsistent(req.Lang, req.AcceptLanguage) {
		score -= deductLangMismatch
	}

	// 屏幕异常小（疑似 headless）
	if w, h, ok := parseScreen(req.Screen); ok {
		if w < screenMinWidth || h < screenMinHeight {
			score -= deductScreenSmall
		}
	}

	// 客户端 ts 与服务端处理间隔过短：真人 onMounted → 1.5s setTimeout 至少 1500ms
	delta := serverNowMs - payload.TS
	if delta >= 0 && delta < tsMinClientDelayMs {
		score -= deductTSTooFast
	}

	// 浏览器专属 Header 缺失计数（每缺失一项扣 deductSecFetchMiss）：
	//   Sec-Fetch-Mode / Sec-Fetch-Site / Sec-Fetch-Dest / Accept-Language
	//
	// 之前 L1 用"缺失 ≥2 直接拒"会误伤 Edge / Brave / Firefox 严格模式
	// （Tracking Prevention 会剥离 Sec-Fetch-Site 等头）。现在改为软扣分，
	// 让真实浏览器即使被剥离 1~2 个头依然能通过 60 分阈值，
	// 而 headless / 脚本通常会同时缺失多个头并叠加其它扣分项被打到 < 60。
	if req.SecFetchMode == "" {
		score -= deductSecFetchMiss
	}
	if req.SecFetchSite == "" {
		score -= deductSecFetchMiss
	}
	if req.SecFetchDest == "" {
		score -= deductSecFetchMiss
	}
	if req.AcceptLanguage == "" {
		score -= deductSecFetchMiss
	}

	if score < scoreThreshold {
		return score, silentReject(reasonL2Score)
	}
	return score, nil
}

// langConsistent 粗粒度比对：取 Lang 主语言段（如 zh-CN → zh），判 Accept-Language 是否包含。
func langConsistent(lang, acceptLang string) bool {
	primary := strings.ToLower(lang)
	if i := strings.Index(primary, "-"); i > 0 {
		primary = primary[:i]
	}
	return strings.Contains(strings.ToLower(acceptLang), primary)
}

// parseScreen 解析 "1920x1080" → (1920, 1080, true)。格式不匹配返回 (0,0,false)。
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

// checkL3 第 3 层：去重 + 频控（均走 Redis）。
//
// 顺序：
//  1. 频控（IP/分钟、IP/小时、fingerprint/分钟）—— 任一超限直接 429
//  2. 主去重 (fp, articleId) SETNX EX 60 —— 命中静默 204
//
// 频控放在前面是为了在攻击者高速刷的场景下尽早返回 429；
// 主去重放在后面以保证用户正常重试 1 分钟后能成功 +1。
func (s *viewLogService) checkL3(ctx context.Context, req *PostViewLogRequest, payload *TokenPayload) *rejectOutcome {
	// 频控：IP/分钟 ≤ 30
	if exceeded, err := s.incrCheck(ctx, "view-log:rate:ip:"+req.IP+":1m", 60, 30); err != nil {
		s.logger.Warn("view-log rate ip:1m incr failed", zap.Error(err))
		// Redis 抖动不应误伤真人，直接放行进入下一阶段
	} else if exceeded {
		return rateLimitedReject(reasonL3RateIP)
	}

	// 频控：IP/小时 ≤ 300
	if exceeded, err := s.incrCheck(ctx, "view-log:rate:ip:"+req.IP+":1h", 3600, 300); err != nil {
		s.logger.Warn("view-log rate ip:1h incr failed", zap.Error(err))
	} else if exceeded {
		return rateLimitedReject(reasonL3RateIP)
	}

	// 频控：fingerprint/分钟 ≤ 10
	if exceeded, err := s.incrCheck(ctx, "view-log:rate:fp:"+payload.FingerprintID+":1m", 60, 10); err != nil {
		s.logger.Warn("view-log rate fp:1m incr failed", zap.Error(err))
	} else if exceeded {
		return rateLimitedReject(reasonL3RateFP)
	}

	// 主去重：(fp, articleId) 1 分钟内只算 1 次
	dedupKey := "view-log:dedup:" + payload.FingerprintID + ":" + payload.ArticleID
	ok, err := s.redis.SetNX(ctx, dedupKey, 1, dedupTTL).Result()
	if err != nil {
		s.logger.Warn("view-log dedup setnx failed", zap.Error(err))
		// 失败时按"未去重"处理，让请求继续；nonce 已经防止 1 分钟内的同请求重放
		return nil
	}
	if !ok {
		return silentReject(reasonL3Dedup)
	}

	return nil
}

// incrCheck INCR + 仅在首次时设置 EXPIRE，返回当前值是否已超阈值。
//
// Redis Pipeline 单 RTT 内完成 INCR 后取计数。EXPIRE 仅在 cnt==1 时设置，
// 避免每次都覆盖 TTL 导致窗口无限延长。
func (s *viewLogService) incrCheck(ctx context.Context, key string, ttlSeconds int, threshold int64) (bool, error) {
	cnt, err := s.redis.Incr(ctx, key).Result()
	if err != nil {
		return false, err
	}
	if cnt == 1 {
		if err := s.redis.Expire(ctx, key, secondsToDuration(ttlSeconds)).Err(); err != nil {
			s.logger.Warn("view-log expire failed", zap.String("key", key), zap.Error(err))
		}
	}
	return cnt > threshold, nil
}
