# 评论审核系统重设计方案

本文重新设计当前网站的评论审核系统。新方案以 `kirklin/go-swd` 作为本地敏感词识别层，第一版优先使用其内置词库，不导入本站旧自定义词库；自定义词库能力保留为空配置，用于后续根据误伤和漏审结果做小范围微调。评论审核应拆成可解释、可配置、可回归测试的流水线。

## 设计目标

- 本地优先：评论提交主链路不能依赖外部审核 API，避免延迟和可用性风险。
- `go-swd` 专职做敏感词识别：第一版使用其内置 7W+ 高质量词条作为主词库，替换当前自研关键词匹配。
- 自定义词库延后启用：保留 `AddWords` 能力，但默认不注入旧词库，避免一开始混用两套词库导致效果无法归因。
- 策略偏严格：内置词库命中默认至少进入待审，高风险分类直接拒绝。
- 决策独立：命中敏感词只是审核信号之一，最终状态由策略引擎统一裁决。
- 配置收敛：线上修复漏洞时只需要判断“词库、结构特征、行为风险、外部兜底”四类入口。
- 可解释：每条待审/拒绝评论都必须记录命中的信号、分类、规则 ID 和最终决策原因。
- 可压测：保留并扩展现有 300 条违规语料，所有规则和词库更新都能跑回归报告。

## 当前系统应废弃的部分

当前 `moderation.go` 同时承担文本归一化、关键词匹配、变体匹配、英文辱骂、正则、base64 URL、涉政组合、安全上下文、行为风险、相似召回、补丁规则和最终评分。这会导致两个问题：

- 每修一个漏审都在一个大文件里继续堆逻辑，长期不可维护。
- `comment_moderation.yml` 的规则语义和执行层绑定太紧，线上不知道应该改词库、改正则、改上下文，还是改相似样本。

新系统应删除这类“一个函数串全部规则”的实现方式，改成模块化信号管线。

## 总体架构

```text
User Submit Comment
        |
        v
Moderation Orchestrator
        |
        +--> Text Normalizer
        |
        +--> Lexicon Layer (go-swd)
        |
        +--> Structure Layer
        |
        +--> Context Layer
        |
        +--> Behavior Layer
        |
        +--> Optional External Review Layer
        |
        v
Policy Decision Engine
        |
        +--> approved
        +--> pending
        +--> rejected
        |
        v
Audit Log + Regression Corpus
```

## 核心接口

审核入口只保留一个统一接口：

```go
type CommentModerator interface {
	Moderate(ctx context.Context, req ModerationRequest) (ModerationResult, error)
}
```

请求结构：

```go
type ModerationRequest struct {
	CommentID uint64
	UserID    uint64
	ArticleID uint64
	ClientIP  string
	Content   string
	Now       time.Time
}
```

结果结构：

```go
type ModerationResult struct {
	Status   string
	Score    int
	Signals  []ModerationSignal
	Decision string
}
```

信号结构：

```go
type ModerationSignal struct {
	Source   string
	Category string
	Level    string
	Score    int
	Reason   string
	Evidence string
	RuleID   string
}
```

`Source` 建议固定为：

- `lexicon`
- `structure`
- `context`
- `behavior`
- `external`
- `manual_feedback`

`Level` 建议固定为：

- `allow`
- `notice`
- `review`
- `block`

这样最终决策不依赖某一个模块的内部实现。

## Layer 1: Text Normalizer

归一化层只做文本视图生成，不做任何审核判断。

输出：

```go
type NormalizedComment struct {
	Raw         string
	Normalized  string
	Compact     string
	AsciiFolded  string
	PinyinFolded string
	DecodedTexts []string
}
```

职责：

- 去除零宽字符和无意义空白。
- 全角转半角。
- 英文大小写统一。
- 生成 compact 文本。
- 提取并解码 base64 片段。
- 生成少量必要的规避视图，例如 `yuepao` -> `约炮`。

注意：`go-swd` README 里提到当前 V1.0 支持基础文本匹配和特殊字符过滤，大小写、全半角、拼音混合、同音、形近仍属于规划项。因此这些规避能力不能完全交给 `go-swd`，需要保留在本项目的 Normalizer 层。

## Layer 2: Lexicon Layer (go-swd)

`go-swd` 是新系统的本地敏感词识别核心。第一版只启用其内置词库，不导入本站原有 `reject/pending` 关键词配置。这样可以先验证 `go-swd` 内置词库在本站评论场景下的真实召回率和误伤率。

README 中的关键用法：

```go
detector, err := swd.New()

customWords := map[string]swd.Category{
	"涉黄": swd.Pornography,
	"涉政": swd.Political,
	"赌博词汇": swd.Gambling,
	"毒品词汇": swd.Drugs,
	"脏话词汇": swd.Profanity,
	"诈骗词汇": swd.Scam,
}

err = detector.AddWords(customWords)
matched := detector.DetectIn(text, swd.Pornography, swd.Political)
words := detector.MatchAll(text)
```

第一版创建 detector 后不调用 `AddWords` 注入旧词表：

```go
detector, err := swd.New()
words := detector.MatchAll(text)
```

`AddWords` 只作为后续微调入口保留。例如线上发现内置词库漏掉少量本站特有风险词，再通过配置注入：

```go
customWords := map[string]swd.Category{
	"本站特有风险词": swd.Custom,
}
err = detector.AddWords(customWords)
```

在本项目中不让 `go-swd` 直接写最终状态，而是做适配：

```go
type LexiconDetector interface {
	Detect(ctx context.Context, text NormalizedComment) ([]ModerationSignal, error)
	Reload(words LexiconWords) error
}
```

分类映射：

| `go-swd` category | 本站审核分类 | 默认处置 |
| --- | --- | --- |
| `swd.Pornography` | `sexual` | `block` |
| `swd.Political` | `political` | `review` |
| `swd.Violence` | `violence` | `review` |
| `swd.Gambling` | `gambling` | `block` |
| `swd.Drugs` | `drugs` | `block` |
| `swd.Profanity` | `abuse` | `review` |
| `swd.Discrimination` | `hate` | `review` |
| `swd.Scam` | `spam_fraud` | `review` |
| `swd.Custom` | `custom` | 由配置指定 |

关键设计：

- `go-swd` 命中只产生 `lexicon` 信号。
- 是否 `pending` 或 `rejected` 由 Policy Decision Engine 决定。
- 内置词库命中默认至少 `review`，即评论进入 `pending`。
- 高风险分类默认 `block`，即评论进入 `rejected`。
- 自定义词库保留，但第一版默认为空；后续不再写成复杂的 `reject/pending` 配置，而是写成“分类 + 等级”。
- 保留 `MatchAll` 的结果作为审核证据，用于后台展示和回归报告。

## Layer 3: Structure Layer

结构层处理不适合词库表达的形态。

典型规则：

- URL、域名、短链。
- 手机号、QQ、微信号、邮箱。
- base64 解码后出现 URL。
- HTML/script 注入。
- 二维码/扫码/私信/加群组合。
- 重复字符、异常标点、广告格式。

结构层配置示例：

```yaml
structure_rules:
  url:
    action: review
  decoded_url:
    action: review
  script_injection:
    action: block
  contact:
    action: review
```

这层应替代当前散落在 `pending.regexps` 里的联系方式和 URL 正则。

## Layer 4: Context Layer

上下文层处理“单个词不违规，但组合起来有风险”的情况。

示例：

- `未成年 + 私照`
- `公共机构 + 冲击`
- `血腥 + 合集`
- `手机号 + 出售`
- `成人资源 + 下载`

新结构：

```yaml
context_rules:
  - id: minor_private_photo
    category: minor
    level: block
    subjects:
      - 未成年
      - 学生
    predicates:
      - 私照
      - 资源
      - 交易
```

与当前 `safety_context` 相比，新增：

- `id`：用于后台证据和回归报告。
- `category`：用于统计风险领域。
- `level`：用于表达 `review/block`，不再依赖全局分数猜测。

## Layer 5: Behavior Layer

行为层只处理用户行为，不看内容语义。

信号：

- 同一用户短时间多次提交。
- 同一 IP 短时间多次提交。
- 同一内容重复提交。
- 新注册用户高频评论。
- 被举报命中率高的用户。

行为层默认只能提升到 `review`，不建议单独 `block`，除非重复刷屏非常明确。

配置示例：

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

## Layer 6: Optional External Review Layer

外部审核不参与主链路阻塞，符合当前项目“外部审核 API 必须异步化”的约束。

策略：

- `approved` 评论可以异步抽检。
- `pending` 评论可以异步请求云审核，结果辅助人工审核。
- `rejected` 评论默认不请求外部审核，节省费用。
- 外部失败或超时不改变主链路结果。

推荐接口：

```go
type ExternalReviewer interface {
	ReviewAsync(ctx context.Context, req ModerationRequest, localResult ModerationResult) error
}
```

## Policy Decision Engine

最终状态由策略引擎统一决定。

建议规则：

1. 任意 `block` 信号命中明确高危分类，状态为 `rejected`。
2. 任意 `go-swd` 内置词库命中，状态至少为 `pending`。
3. 任意 `review` 信号命中，状态至少为 `pending`。
4. 多个弱信号累计超过阈值，状态为 `pending`。
5. 没有风险信号，状态为 `approved`。
6. 审核模块异常时，状态为 `pending`，不能默认放行。

示例配置：

```yaml
decision:
  default_on_error: pending
  levels:
    block: rejected
    review: pending
  score:
    pending: 40
    rejected: 80
  category_overrides:
    sexual:
      level: block
    gambling:
      level: block
    drugs:
      level: block
    political:
      level: review
    abuse:
      level: review
```

这里保留分数，但分数只是排序和统计，不再作为唯一决策来源。

## 配置重构

建议把 `comment_moderation.yml` 收敛为以下结构：

```yaml
comment_moderation:
  disabled: false

  lexicon:
    provider: go_swd
    use_builtin: true
    strict_builtin_match: true
    custom_words:
      block: {}
      review: {}

  structure_rules:
    decoded_url:
      level: review
    contact:
      level: review
    script_injection:
      level: block

  context_rules:
    - id: illegal_privacy_trade
      category: illegal_privacy
      level: review
      subjects:
        - 身份证
        - 银行卡
        - 手机号
      predicates:
        - 出售
        - 买卖
        - 查询

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

  decision:
    default_on_error: pending
    score:
      pending: 40
      rejected: 80
```

线上修复入口变成：

| 线上问题 | 修改位置 |
| --- | --- |
| `go-swd` 内置词库已命中，但处置过松或过严 | `decision.category_overrides` |
| `go-swd` 内置词库漏掉明确敏感词 | `lexicon.custom_words.block` |
| `go-swd` 内置词库漏掉可疑词 | `lexicon.custom_words.review` |
| 新增 URL/联系方式/脚本类形态 | `structure_rules` |
| 新增组合语义风险 | `context_rules` |
| 刷屏/重复提交问题 | `behavior_rules` |
| 决策过松或过严 | `decision` |

不再保留 `reject/pending/similarity/political_context/safety_context` 这种混合结构。

第一版上线时，`lexicon.custom_words` 应保持为空。只有当回归报告和人工审核结果证明内置词库存在稳定漏审时，才补充自定义词库。

## 包结构建议

```text
app/service/comment/moderation/
├── moderator.go        # Orchestrator
├── types.go            # ModerationRequest/Result/Signal
├── normalizer.go       # Text Normalizer
├── lexicon.go          # LexiconDetector interface
├── lexicon_swd.go      # go-swd adapter
├── structure.go        # URL/contact/script/base64
├── context.go          # subject + predicate rules
├── behavior.go         # Redis-backed behavior signals
├── decision.go         # Policy Decision Engine
├── audit.go            # audit log/event model
└── testdata/
```

旧的 `app/service/comment/moderation.go` 应拆掉，不再保留一个大文件。

## 数据库与审计

当前 `comment` 表只存最终状态，不足以分析漏审。建议新增审核记录表：

```text
comment_moderation_audit
  id
  comment_id
  status
  score
  decision
  signals_json
  config_version
  create_time
```

用途：

- 管理后台展示“为什么进入待审/拒绝”。
- 回放线上漏审样本。
- 对比配置变更前后的审核结果。
- 统计各分类命中率和误伤率。

## 管理后台配合

后台评论列表应展示：

- 最终状态。
- 风险分类。
- 命中来源：`lexicon/structure/context/behavior/external`。
- 命中证据：敏感词、规则 ID、结构规则名称。
- 人工处理结果。

人工处理应反哺：

- 人工驳回 pending -> 进入误伤样本集。
- 人工拒绝 pending -> 进入漏审强化样本集。
- 多次命中的新词 -> 提醒加入 `lexicon.custom_words`。

## 回归测试

保留现有语料测试，但报告格式改成信号级：

```text
TOTAL cases=300 approved=0 pending=190 rejected=110

TOP SIGNALS
lexicon:gambling:block = 32
structure:decoded_url:review = 12
context:illegal_privacy:review = 9
behavior:duplicate_content:review = 6
```

每次改配置或升级 `go-swd` 后必须跑：

```bash
go test ./app/service/comment -run TestCommentModerationReportCorpus -v
```

新增 PoC 测试：

- 对比当前实现和新实现的 300 条语料结果。
- 统计新增 `go-swd` 后的误伤样本。
- 单独测试 `go-swd` 内置词库在本站正常评论上的误伤率。
- 单独统计 `go-swd` 内置词库命中后的分类分布，确认哪些分类应该直接拒绝，哪些分类只进入待审。
- 自定义词库保持为空跑第一轮报告，避免把旧词库效果混入基线。

## 迁移步骤

1. 引入 `github.com/kirklin/go-swd`，先写 `LexiconDetector` 适配层和单测。
2. 新增 `moderation` 子包，实现 `types.go`、`normalizer.go`、`decision.go`。
3. 第一版只启用 `go-swd` 内置词库，自定义词库配置保持为空。
4. 设置偏严格决策：内置词库命中至少 `pending`，`sexual/gambling/drugs` 等高风险分类直接 `rejected`。
5. 把 URL、联系方式、base64、script 检测迁入 `structure.go`。
6. 把 `safety_context` 重写为带 `id/category/level` 的 `context_rules`。
7. 把 Redis 行为风险迁入 `behavior.go`。
8. 新增审核审计表和写入逻辑。
9. 改 `commentService.UserAddComment` 只调用新的 `CommentModerator`。
10. 删除旧 `moderation.go` 和旧配置字段。
11. 用现有 300 条语料、正常评论语料和线上漏审样本做 A/B 报告。
12. 确认内置词库的误伤和漏审分布后，再决定是否补充 `lexicon.custom_words`。

## 最终结论

`go-swd` 应作为新系统的敏感词基础设施，而不是完整审核系统。适合当前网站的方案是：

- `go-swd` 负责高性能本地词库识别，第一版优先使用内置词库作为主审核词库。
- 自定义词库功能保留，但默认为空，只用于后续微调。
- 本项目保留文本归一化、结构检测、上下文组合、行为风险。
- 决策引擎统一把信号转换成 `approved/pending/rejected`，默认策略偏严格。
- 配置从“按技术实现分组”改成“按线上处理动作分组”。
- 所有结果写审计日志，并通过语料回归验证。

这比继续维护现有 `moderation.go` 更稳，也比直接把评论审核交给开源库更可控。
