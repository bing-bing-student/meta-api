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
- 机制与策略分离：Unicode、URL、手机号、脚本等检测机制保留在代码；业务词族、组合规则、语义上下文和处置策略进入配置。
- 保守放行：语义复判按证据所在片段执行，不允许一段良性描述压制另一段真实推广。

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
        +--> Policy Compiler (validate + normalize + cache)
        +--> Text Normalizer
        +--> Clause Segmenter
        +--> Lexicon Layer (go-swd + custom words)
        +--> Structure Layer
        +--> Combination Rule Layer
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
- `app/service/comment/moderation/policy.go`
- `app/service/comment/moderation/normalizer.go`
- `app/service/comment/moderation/clause.go`
- `app/service/comment/moderation/lexicon_swd.go`
- `app/service/comment/moderation/structure.go`
- `app/service/comment/moderation/combination.go`
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
	Trace    Trace
}
```

`Trace` 只用于解释审核过程，不参与最终决策：

```go
type Trace struct {
	Clauses           []ClauseTrace
	DetectorSignals   []Signal
	SuppressedSignals []Signal
	BehaviorEvaluated bool
}
```

- `DetectorSignals`：词库、相似度、结构、组合和行为层产生的原始信号。
- `SuppressedSignals`：被片段级良性语境或普通辱骂策略抑制的信号。
- `Result.Signals`：语义修正后进入决策引擎的最终信号。

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
	Clause   int
}
```

`Clause` 为从 1 开始的语义片段编号；`0` 表示信号属于整条评论或无法安全定位到单一片段。

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
- 生成 Unicode confusable skeleton 视图，折叠常见西里尔、希腊同形字符。

## Layer 2: Lexicon Layer

词库层使用 `github.com/kirklin/go-swd` 作为本地敏感词基础设施。

当前策略：

- 启用 `go-swd` 内置词库。
- 启用 `lexicon.custom_words` 做本站场景微调。
- 高风险分类如 `sexual`、`gambling` 默认 `block`。
- `abuse`、`spam_fraud`、`sensitive` 等默认 `review`。
- 短 ASCII 词命中会做边界过滤，避免 `Java` 中的 `av` 误杀。
- 部分中文技术术语做精确跳过，例如 `垃圾回收` 中的 `垃圾` 不作为辱骂命中。

`go-swd v0.0.3` 已确认可用的高级能力：

- AC 自动机多模式匹配。
- 敏感词分类筛选和匹配位置。
- 并发安全检测。
- 自定义词批量添加、删除和动态重建词典。
- 大小写、空白、全半角和数字样式预处理器。

该版本虽然暴露了拼音、同音字、形近字、`MaxDistance`、URL 和邮箱等选项，但检测器源码没有消费大部分字段，而且实例创建后的 option setter 不会重建预处理器。因此这些接口不能作为已实现能力依赖。

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
  fuzzy:
    disabled: false
    max_distance: 1
    min_word_runes: 4
    candidate_words:
      spam_fraud: []
```

治理原则：

- 明确违法、高风险内容可放入 `block`。
- 需要人工判断或容易误伤的内容放入 `review`。

## Similarity Layer

相似度层只补充精确词库无法覆盖的局部变体，不独立拒绝评论。

当前算法：

- **Unicode Confusable Skeleton**：将常见西里尔、希腊同形字符映射到稳定骨架，并作为额外词库视图。
- **受限加权编辑距离**：只扫描 `lexicon.fuzzy.candidate_words` 中人工确认的候选词；候选长度、最大距离均受配置限制。
- **片段定位**：模糊信号携带 `Clause`，因此引用、批判和技术语境仍可由统一语义层抑制。

禁止将编辑距离直接应用到整个内置词库。短词和无限候选模糊匹配会显著放大误杀。

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

协议和格式识别由 Go 代码实现，运营型风险短语由
`structure_patterns.risk_phrases/risk_patterns` 配置。

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

## Layer 4: Combination Rule Layer

组合规则层处理“单个词未必违规，但主体与动作在同一片段出现时具有风险”的情况。原先代码内的隐式意图规则已经与配置规则合并，只保留一个执行器。

当前配置格式：

```yaml
combination_rules:
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

执行约束：

- 主体词和动作词必须出现在同一语义片段。
- 每条规则必须有唯一 `id`、非空主体词和非空动作词。
- 信号直接记录片段编号，后续语义修正不再通过证据字符串猜测位置。
- 配置错误会使审核进入 `default_on_error`，不会静默跳过错误规则。

当前主要覆盖：

- 涉政动员
- 未成年人风险
- 暴力邀约
- 血腥内容交易
- 色情擦边组合
- 商业引流和灰产交易
- 隐私查询和非法账号交易
- 受限数字资产、内部数据和审计规避
- 攻击工具、凭据窃取和攻击服务
- 编码、拆词及跨页面联系方式引流
- 不良价值导向

联系方式规避不依赖单一平台名称。系统组合识别“编码方式或承载位置”与“搜索、拼接、扫码、回复后联系”等动作；URL 分段发送另由结构正则补充。

上下文信号的 `RuleID` 使用配置里的 `id`，便于后台展示和报告统计。

## Layer 5: Behavior Layer

行为层只处理用户行为，不理解内容语义。

当前信号：

- `user_frequency`
- `ip_frequency`
- `duplicate_content`
- `near_duplicate`

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
  near_duplicate:
    disabled: false
    window_seconds: 86400
    review_threshold: 2
    max_hamming_distance: 10
    min_content_runes: 12
    max_samples: 100
    max_length_difference_percent: 30
```

实现说明：

- Redis ZSet 记录用户和 IP 的窗口内评论行为。
- Redis counter 记录重复内容。
- SimHash 使用二字字符特征，比较同一用户在同一文章下的近期评论。
- 近重复必须同时满足汉明距离和文本长度差约束，并且只产生 `review`。
- 正常用户提交评论后会记录行为。
- 后台审核模拟使用 dry-run，不写行为统计；只有提供用户、文章或 IP 上下文时才只读执行行为检测。

## 后台审核模拟

`POST /admin/auth/comment/moderation-preview` 保持原有状态、分数和原因字段，并额外返回：

- 规范化、混淆骨架、拼音折叠和解码文本视图。
- 语义片段列表。
- 探测器原始信号、语义抑制信号和最终信号。
- 每个信号的来源、分类、等级、规则 ID、证据和片段编号。
- 行为层是否实际执行。

`admin-ui` 可以提供可选的 `userID`、`articleID` 和 `clientIP`。任一上下文存在时，接口读取当前 Redis 行为状态，但不会调用行为记录逻辑。

## Layer 6: Semantic Adjustment

语义复判层用于降低关键词裸匹配带来的误杀，不是大模型语义理解。每个片段会识别以下上下文：

- `reporting`：新闻、研究、判决书、治理报告等引用或陈述。
- `rejection`：拒绝、举报、警示、禁止和治理表达。
- `technical`：文档、测试、代码、医学和安全研究语境。
- `actionable`：第一人称提供、交易、引流和实际执行意图。
- `abuse severity`：普通尖锐观点可放行，严重人格羞辱、骚扰动员和排斥行为保留待审。

`actionable` 优先级高于一般良性上下文。行为风险、脚本注入、解码 URL 和文本质量信号不可被良性语境压制。

典型放行场景：

- 引用、讨论、反诈骗、测试、误杀分析。
- 技术语境，例如 `垃圾回收`。
- 自谦语境，例如 `初来乍到，今天献丑了`。
- 对文章、教程、代码和作者能力的尖锐批评或讽刺。

典型不放行场景：

- `私密资源要不要？加好友发你，别在评论区问`
- `谁有办法查一个人的手机号、住址和开房记录？有偿提供`
- 严重智力羞辱、家属攻击、组织举报、开盒、围攻或行业封杀。
- 实际推广使用的联系方式、URL、上下文组合风险和行为风险。

配置入口：

```yaml
semantic_rules:
  disabled: false
  contexts:
    reporting_markers: []
    rejection_markers: []
    technical_markers: []
    unambiguous_benign_markers: []
    actionable_markers: []
    actionable_patterns: []
  abuse_policy:
    disabled: false
    severe_markers: []
```

`abuse_policy` 只修正 `category=abuse` 的信号，不会压制诈骗、色情、赌博、越权或行为信号。严重标记按信号所在片段判断，避免其他片段的尖锐观点掩盖真实攻击。

## Policy Compiler

原始 YAML 不直接进入检测器。`policyCache.Resolve` 会执行：

1. 生成配置签名，配置没有变化时复用已编译策略。
2. 校验组合规则 ID、主体词、动作词和处置等级。
3. 校验所有可配置正则。
4. 使用与评论相同的规则归一化配置词项。
5. 缓存不可变策略；热更新后签名变化才重新编译。

这避免了每条评论重复归一化上千个配置词，同时保证错误配置不会静默漏审。

## 可复用治理方法

审核能力按以下闭环维护：

1. **标准化输入**：先处理 Unicode、零宽字符、全半角、数字和有限混淆字符。
2. **片段化理解**：主体、动作和语义上下文只在同一片段组合。
3. **多层取证**：词库、结构、组合规则和用户行为各自产生可解释信号。
4. **上下文修正**：只修正证据所在片段，实际提供意图优先于良性包装。
5. **统一决策**：规则只产出信号，最终状态由决策引擎统一计算。
6. **黄金集回归**：每次策略变更都运行正常、违规和灰度语料。
7. **盲测与线上反馈**：参与调优的黄金集用于防回退，独立盲测和线上人工结果用于评估泛化能力。

新增规则时优先扩展已有词族和组合关系；只有检测机制变化时才修改 Go 代码。

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
+ similarity
+ structure
+ combination rules
+ behavior
+ semantic adjustment
+ decision
```

这套架构适合当前 1C/2G 的线上资源约束：主链路不依赖外部服务，规则可解释，治理成本可控。后续如果要继续提升效果，优先方向不是推翻架构，而是补充语料、优化词库边界、细化上下文规则，并在必要时为灰区样本增加轻量语义分类器。
