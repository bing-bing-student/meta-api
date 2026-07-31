# 评论审核系统架构说明

本文记录当前已落地的评论审核架构。评论审核现在是本地规则优先、可解释、可配置、可回归测试的信号管线，最终对外仍只输出三种状态：

- `approved`
- `pending`
- `rejected`

内部通过 `risk_score`、命中信号和中文原因辅助治理，但前台和后台业务状态不扩展新枚举。

## 设计目标

- 本地优先：评论提交主链路不依赖外部审核 API，避免延迟和可用性风险。
- 可解释：待审和拒绝结果要能展示命中来源、分类、规则 ID、证据和中文原因。
- 可配置：线上治理优先修改 `config/comment_moderation.yml`，避免频繁改代码。
- 可回归：通过 `testdata` 语料持续验证漏审和误杀。
- 可干预：后台提供审核模拟能力，管理员可以 dry-run 多条评论，不创建评论、不写行为统计。
- 保守放行：语义复判只压制低风险词库/短语误杀，不压制联系方式、URL、上下文组合、行为风险等强风险信号。

## 总体链路

```text
User Submit Comment
        |
        v
commentService.UserAddComment
        |
        v
moderation.Moderator
        |
        +--> Text Normalizer
        +--> Lexicon Layer (go-swd + custom words)
        +--> Structure Layer
        +--> Context Layer
        +--> Behavior Layer (Redis)
        +--> Semantic Adjustment
        |
        v
Policy Decision Engine
        |
        +--> approved
        +--> pending
        +--> rejected
        |
        v
Persist Comment + Moderation Reasons
```

主要代码位置：

- `app/service/comment/moderation/moderator.go`
- `app/service/comment/moderation/normalizer.go`
- `app/service/comment/moderation/lexicon_swd.go`
- `app/service/comment/moderation/structure.go`
- `app/service/comment/moderation/context.go`
- `app/service/comment/moderation/behavior.go`
- `app/service/comment/moderation/semantic.go`
- `app/service/comment/moderation/decision.go`
- `config/comment_moderation.yml`

## 核心数据结构

审核请求：

```go
type Request struct {
	CommentID uint64
	UserID    uint64
	ArticleID uint64
	ClientIP  string
	Content   string
	Now       time.Time
}
```

审核结果：

```go
type Result struct {
	Status   string
	Score    int
	Signals  []Signal
	Reasons  []string
	Decision string
}
```

审核信号：

```go
type Signal struct {
	Source   string
	Category string
	Level    string
	Score    int
	Reason   string
	Evidence string
	RuleID   string
}
```

当前 `Source`：

- `lexicon`
- `structure`
- `context`
- `behavior`
- `semantic`

当前 `Level`：

- `allow`
- `notice`
- `review`
- `block`

## Layer 1: Text Normalizer

归一化层只生成文本视图，不直接决定审核状态。

输出：

```go
type NormalizedComment struct {
	Raw          string
	Normalized   string
	Compact      string
	PinyinFolded string
	DecodedTexts []string
}
```

当前能力：

- 去除零宽字符、控制字符和无意义空白。
- 全角转半角。
- 英文统一小写。
- 生成 `compact` 文本。
- 生成少量拼音折叠视图，例如 `yuepao` -> `约炮`。
- 提取并解码 base64 URL 候选。
- 数字样式归一化，例如 `9⓿二肆⁹₈` -> `902498`。
- 中文数字在数字串上下文中归一化，避免全局替换破坏正常中文句子。

## Layer 2: Lexicon Layer

词库层使用 `github.com/kirklin/go-swd` 作为本地敏感词基础设施。

当前策略：

- 启用 `go-swd` 内置词库。
- 启用 `lexicon.custom_words` 做本站场景微调。
- 高风险分类如 `sexual`、`gambling` 默认 `block`。
- `abuse`、`spam_fraud`、`sensitive` 等默认 `review`。
- 短 ASCII 词命中会做边界过滤，避免 `Java` 中的 `av` 误杀。
- 部分中文技术术语做精确跳过，例如 `垃圾回收` 中的 `垃圾` 不作为辱骂命中。

配置入口：

```yaml
lexicon:
  provider: go_swd
  use_builtin: true
  strict_builtin_match: true
  custom_words:
    block:
      sexual: []
      gambling: []
    review:
      abuse: []
      spam_fraud: []
```

治理原则：

- 明确违法、高风险内容可放入 `block`。
- 需要人工判断或容易误伤的内容放入 `review`。

## Layer 3: Structure Layer

结构层处理不适合词库表达的形态风险。

当前规则：

- `url`
- `decoded_url`
- `contact`
- `script_injection`
- `text_quality`
- `risk_phrase`

典型能力：

- URL、域名、混写 URL。
- 手机号、QQ、微信、加好友、私信、进群等联系方式意图。
- 邮箱混淆写法。
- base64 解码后 URL。
- script 注入。
- 纯数字、数字样式串、纯符号、重复字符等低质文本。
- 广告、诈骗、暴力、低俗等风险短语。

结构规则的默认等级来自：

```yaml
structure_rules:
  url:
    level: review
  decoded_url:
    level: review
  contact:
    level: review
  script_injection:
    level: block
  text_quality:
    level: review
  risk_phrase:
    level: review
```

## Layer 4: Context Layer

上下文层处理“单个词未必违规，但组合起来有风险”的情况。

当前配置格式：

```yaml
context_rules:
  - id: illegal_privacy
    category: illegal_privacy
    level: review
    subjects:
      - 手机号
      - 住址
      - 开房记录
    predicates:
      - 查
      - 有偿
      - 有偿提供
```

当前主要覆盖：

- 涉政动员
- 未成年人风险
- 暴力邀约
- 血腥内容交易
- 色情擦边组合
- 商业引流和灰产交易
- 隐私查询和非法账号交易
- 不良价值导向

上下文信号的 `RuleID` 使用配置里的 `id`，便于后台展示和报告统计。

## Layer 5: Behavior Layer

行为层只处理用户行为，不理解内容语义。

当前信号：

- `user_frequency`
- `ip_frequency`
- `duplicate_content`

配置：

```yaml
behavior_rules:
  user_frequency:
    window_seconds: 600
    review_threshold: 6
  ip_frequency:
    window_seconds: 600
    review_threshold: 12
  duplicate_content:
    window_seconds: 86400
    review_threshold: 2
    block_threshold: 4
```

实现说明：

- Redis ZSet 记录用户和 IP 的窗口内评论行为。
- Redis counter 记录重复内容。
- 正常用户提交评论后会记录行为。
- 后台审核模拟使用 dry-run，不写行为统计。

## Layer 6: Semantic Adjustment

语义复判层用于降低关键词裸匹配带来的误杀，不是大模型语义理解。

当前策略：

- 命中良性语境 marker 时，可以压制低风险词库或 `risk_phrase` 信号。
- 如果存在强风险信号，则不做压制。
- 强风险包括：
  - `behavior`
  - `context`
  - `structure.contact`
  - `structure.url`
  - `structure.decoded_url`
  - `structure.script_injection`

典型放行场景：

- 引用、讨论、反诈骗、测试、误杀分析。
- 技术语境，例如 `垃圾回收`。
- 自谦语境，例如 `初来乍到，今天献丑了`。

典型不放行场景：

- `私密资源要不要？加好友发你，别在评论区问`
- `谁有办法查一个人的手机号、住址和开房记录？有偿提供`
- 任何带联系方式、URL、上下文组合风险、行为风险的评论。

配置入口：

```yaml
semantic_rules:
  disabled: false
  benign_context:
    suppress_sources:
      - lexicon
    suppress_rule_ids:
      - risk_phrase
    suppress_categories:
      - "*"
    markers: []
```

## Policy Decision Engine

最终状态仍然只有三种。

当前决策规则：

1. 任意 `block` 信号命中，状态为 `rejected`。
2. 任意 `review` 信号命中，状态为 `pending`。
3. 没有风险信号，状态为 `approved`。
4. 审核模块异常时，默认 `pending`。
5. `review` 信号可以累计风险分，但累计到拒绝阈值时会压到 `reject - 1`，避免多个待审信号自动升级成拒绝。

默认阈值：

```yaml
decision:
  default_on_error: pending
  score:
    pending: 40
    reject: 80
```

评分细化：

```yaml
decision:
  rule_scores:
    review: 40
    block: 80
    "lexicon:spam_fraud:review": 60
    "context:illegal_privacy:illegal_privacy:review": 60
```

风险分语义：

- `0`：明确正常。
- `40`：普通待审。
- `60`：高风险待审。
- `80`：直接拒绝。
- `79`：多个高风险待审信号叠加后仍保持 `pending`。

注意：`risk_score` 用于后台解释、排序和治理分析，不改变对外状态枚举。

## 评论提交与持久化

前台评论提交链路：

```text
UserAddComment
  -> moderateComment
  -> CreateComment
  -> recordCommentModerationBehavior
```

`comment` 表当前新增：

```go
ModerationReasons string `gorm:"column:moderation_reasons;type:text"`
```

数据库中保存 raw reason，例如：

```text
lexicon:abuse:review:骗子
context:illegal_privacy:review:illegal_privacy:手机号+查
```

后台返回时会格式化成中文原因，例如：

```text
敏感词库命中：骗子（辱骂攻击，待人工复核）
上下文规则命中：手机号+查（隐私与非法交易风险，待人工复核）
```

## 举报与限流

举报阈值：

```yaml
report_threshold: 3
```

含义：同一评论累计 `pending` 举报数达到阈值后，如果评论当前是 `approved`，会转为 `pending`。

为降低滥用，举报有独立限流：

```yaml
rate_limit:
  comment_report:
    ip:
      limit: 30
      window_seconds: 86400
    user:
      limit: 10
      window_seconds: 86400
    ip_comment:
      limit: 2
      window_seconds: 86400
```

## 回归测试集

当前新增了长期维护的黄金测试集：

```text
app/service/comment/testdata/comment_moderation/
  normal.tsv       # 非违规评论，期望 approved
  violation.tsv    # 违规评论，期望 pending 或 rejected
  gray.tsv         # 灰区样本，只生成报告，不阻塞测试
  candidates.tsv   # 线上候选样本池，人工确认前不进入强断言
  report.txt       # normal + violation 强回归报告
  gray_report.txt  # gray 灰区观察报告
```

TSV 字段：

```text
id	text	expected	category	tags	note
```

`expected` 支持：

- `approved`：必须通过，否则计入误杀。
- `risk`：只要不是 `approved` 即视为拦住，`pending` 和 `rejected` 都可接受。
- `pending`：必须待审核。
- `rejected`：必须拒绝。

核心指标：

```text
误杀率 = normal.tsv 中 actual != approved 的数量 / normal.tsv 总数
漏审率 = violation.tsv 中 actual == approved 的数量 / violation.tsv 总数
```

当前强回归阈值：

```text
误杀率 = 0
漏审率 = 0
```

样本流转规则：

1. 线上新发现的评论先放入 `candidates.tsv`。
2. 人工确认后再移动到 `normal.tsv`、`violation.tsv` 或 `gray.tsv`。
3. 每次改算法或配置，都要看 `report.txt` 中误杀率、漏审率和错误分类。
4. `gray.tsv` 不阻塞测试，只用于观察灰区策略是否符合预期。

常用命令：

```bash
go test ./app/service/comment -run 'TestCommentModerationGoldenCorpus|TestCommentModerationGrayCorpusReport' -v
go test ./...
```

## 当前结论

`go-swd` 现在是敏感词基础设施，不是完整审核系统。真正的审核能力来自多层信号组合：

```text
normalizer
+ lexicon
+ structure
+ context
+ behavior
+ semantic adjustment
+ decision
```

这套架构适合当前 1C/2G 的线上资源约束：主链路不依赖外部服务，规则可解释，治理成本可控。后续如果要继续提升效果，优先方向不是推翻架构，而是补充语料、优化词库边界、细化上下文规则，并在必要时为灰区样本增加轻量语义分类器。
