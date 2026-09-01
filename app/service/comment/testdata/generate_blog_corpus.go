//go:build ignore

// Command generate_blog_corpus deterministically builds the large, blog-specific
// moderation regression corpus. The compact hand-written corpus remains the place
// for individually diagnosed bugs; this generator covers broad scenario matrices.
package main

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
)

type corpusRow struct {
	ID       string
	Text     string
	Expected string
	Category string
	Tags     string
	Note     string
}

type violationFamily struct {
	Category string
	Tags     string
	Subjects []string
	Actions  []string
	Endings  []string
	Build    func(subject, action, ending string) string
}

func main() {
	directory := filepath.Dir(os.Args[0])
	if len(os.Args) > 1 {
		directory = os.Args[1]
	}
	mustWrite(filepath.Join(directory, "normal_blog_generated.tsv"), buildNormalRows())
	mustWrite(filepath.Join(directory, "violation_blog_generated.tsv"), buildViolationRows())
}

func buildNormalRows() []corpusRow {
	topics := []string{
		"Go 并发控制", "Go 垃圾回收", "Java 虚拟机", "Spring 事务", "MySQL 索引",
		"MySQL 事务隔离", "Redis 缓存", "Redis 主从复制", "Docker 镜像", "Docker 网络",
		"Kubernetes 调度", "Kubernetes 探针", "Nginx 反向代理", "Linux 进程", "Linux 文件权限",
		"Git 分支管理", "Git 合并冲突", "CI 构建流水线", "Vue 响应式", "Vue 组件通信",
		"React 状态管理", "TypeScript 类型系统", "JavaScript 事件循环", "CSS 布局", "浏览器渲染",
		"HTTP 缓存", "HTTPS 证书", "WebSocket 通信", "消息队列", "分布式锁",
		"微服务链路追踪", "限流算法", "数据库连接池", "全文搜索", "对象存储",
		"日志采集", "监控告警", "单元测试", "接口设计", "系统架构",
	}
	observations := []string{
		"概念解释很清楚", "示例代码容易跟着复现", "流程图帮助我理清了调用顺序", "边界条件总结得很完整",
		"性能对比给出了有用依据", "故障排查步骤很有参考价值", "配置示例与实际项目很接近", "常见误区分析得很到位",
		"从原理到实践的衔接很自然", "关键参数的说明足够具体", "异常处理部分写得很细致", "文章结构读起来很顺畅",
	}
	endings := []string{
		"我准备在本地项目里验证一下。",
		"期待后续继续补充实战细节。",
	}
	questions := []string{
		"能否补充一个高并发场景下的完整示例？",
		"如果运行环境发生变化，这里的参数应该怎样调整？",
		"这一方案在数据量增大后还需要注意哪些边界？",
		"后续可以对比一下其他实现方式的取舍吗？",
		"出现超时以后，建议优先检查哪些日志指标？",
	}
	questionContexts := []string{
		"我读完关于%s的章节后还有一个问题：%s",
		"正在把%s用到自己的博客项目里，想请教：%s",
	}

	rows := make([]corpusRow, 0, 1520)
	appendRow := func(text, category, tags, note string) {
		rows = append(rows, corpusRow{
			ID: fmt.Sprintf("BN%04d", len(rows)+1), Text: text, Expected: "approved",
			Category: category, Tags: tags, Note: note,
		})
	}
	for _, topic := range topics {
		for _, observation := range observations {
			for _, ending := range endings {
				appendRow(fmt.Sprintf("这篇关于%s的文章%s，%s", topic, observation, ending),
					"technical_blog", "blog,technical,reader_feedback", "normal technical blog feedback")
			}
		}
		for _, question := range questions {
			for _, context := range questionContexts {
				appendRow(fmt.Sprintf(context, topic, question),
					"technical_question", "blog,technical,question", "normal technical question")
			}
		}
	}

	riskExamples := []string{
		"有资源的丝聊我", "低价出售账号", "论文代写", "成人网站会员", "帮查个人信息",
		"保证收益", "赌博平台", "加群领取资料", "刷评论", "未成年人私照",
	}
	defensiveTemplates := []string{
		"这篇文章把%s作为违规评论示例，提醒读者不要模仿。",
		"审核系统应该识别%s这类表达，但不能误伤科普讨论。",
		"我反对%s这种行为，发现后应当及时举报。",
		"安全测试正在验证%s是否会触发复核。",
		"作者引用%s是为了分析不良评论的传播方式。",
		"请不要发布%s，正常问题直接在公开评论区讨论。",
		"平台治理文档记录了%s的处置案例。",
		"如果有人发布%s，管理员应该怎样处理？",
	}
	quoteStyles := []func(string) string{
		func(value string) string { return "“" + value + "”" },
		func(value string) string { return "‘" + value + "’" },
	}
	for _, example := range riskExamples {
		for _, template := range defensiveTemplates {
			for _, quote := range quoteStyles {
				appendRow(fmt.Sprintf(template, quote(example)), "moderation_discussion",
					"blog,moderation,quote,counter_evidence", "quoted or rejected risk example")
			}
		}
	}

	harmfulTopics := []string{
		"法轮功", "邪教", "赌博平台", "网络霸凌", "校园欺凌", "敌敌畏",
		"百草枯", "跳楼", "割腕", "自杀", "仇恨言论", "未成年人私照",
	}
	educationTemplates := []string{
		"今天上课老师科普%s的知识，让我们了解其危害。",
		"学校课程介绍%s的风险，提醒学生远离。",
		"教材讲解%s的危害，帮助大家形成防范意识。",
		"这篇文章分析%s造成的风险，并提醒读者及时举报。",
		"新闻通报%s的危害，呼吁公众提高警惕。",
		"平台通报%s属于违规行为，并介绍举报方式。",
		"老师在课堂上批判%s，说明这种行为十分危险。",
		"医生讲解%s的中毒风险，提醒误触后立即就医。",
		"警方通报%s的处置案例，提醒大家及时报警。",
		"课程分析%s的违法风险，要求学生不要模仿。",
	}
	for _, topic := range harmfulTopics {
		for _, template := range educationTemplates {
			appendRow(fmt.Sprintf(template, topic), "risk_education",
				"blog,education,counter_evidence,relation", "actor-action-risk-critical-outcome counter-evidence")
		}
	}

	for _, action := range []string{"跳楼", "割腕", "上吊", "服毒", "自杀", "卧轨", "摸高压线", "闯红灯"} {
		for _, template := range []string{
			"请勿%s，遇到困难应及时求助。",
			"如果发现有人准备%s，应立即阻止并报警。",
			"警方救下准备%s的人，并安排了心理救助。",
			"老师提醒同学不要%s，这种行为非常危险。",
			"这篇文章分析%s事件的原因，强调应及时防范。",
		} {
			appendRow(fmt.Sprintf(template, action), "danger_prevention",
				"blog,safety,prevention,relation", "dangerous action prevention or reporting")
		}
	}

	for _, substance := range []string{"敌敌畏", "百草枯", "农药", "鼠药"} {
		for _, template := range []string{
			"%s有毒，误服后应立即就医。",
			"医生提醒不要吞%s，一旦误食需要立即求助。",
			"学校科普%s的中毒风险，要求学生远离。",
		} {
			appendRow(fmt.Sprintf(template, substance), "poison_prevention",
				"blog,safety,poison,prevention", "dangerous substance prevention")
		}
	}
	return rows
}

func buildViolationRows() []corpusRow {
	families := []violationFamily{
		{
			Category: "spam_fraud", Tags: "blog,spam,resource_diversion",
			Subjects: []string{"课程资料", "内部资料", "资源", "账号", "平台会员", "游戏账号", "论文", "毕业设计", "直播间人气", "评论区互动", "网站地址", "私密资源"},
			Actions:  []string{"私聊", "私信", "看我主页", "进群", "低价出售", "发你", "扫码领取", "加好友", "按条收费", "走老方式"},
			Endings:  []string{"需要的直接说。", "名额不多先到先得。", "别在公开评论区细问。", "长期稳定可以安排。"},
			Build: func(subject, action, ending string) string {
				return fmt.Sprintf("我这里有%s，可以%s，%s", subject, action, ending)
			},
		},
		{
			Category: "illegal_privacy", Tags: "blog,privacy,illegal_service",
			Subjects: []string{"身份证", "银行卡", "账号密码", "住址电话", "个人隐私", "开房记录", "手机号", "通话记录", "手机定位", "数据库账号"},
			Actions:  []string{"帮查", "查询", "有偿提供", "出售", "弄到", "私聊提供", "收费查询", "通过渠道拿到"},
			Endings:  []string{"价格可以商量。", "只接熟人介绍。", "需要的私下联系。"},
			Build: func(subject, action, ending string) string {
				return fmt.Sprintf("我这里可以%s%s，%s", action, subject, ending)
			},
		},
		{
			Category: "sexual", Tags: "blog,sexual,promotion",
			Subjects: []string{"成人资源", "成人内容", "成人视频", "成人影片", "成人交友", "刺激内容", "特殊服务", "裸聊"},
			Actions:  []string{"私发", "提供下载", "发链接", "拉你进群", "私信报价", "便宜出", "给地址"},
			Endings:  []string{"未满不要来。", "不要在这里公开问。", "看完记得删记录。"},
			Build: func(subject, action, ending string) string {
				return fmt.Sprintf("这边有%s，可以%s，%s", subject, action, ending)
			},
		},
		{
			Category: "academic_cheating", Tags: "blog,academic_cheating,paid_service",
			Subjects: []string{"论文", "实验报告", "课程设计", "毕业设计", "项目代做", "作业", "答辩稿", "选题"},
			Actions:  []string{"代做", "帮你弄完", "直接帮你", "保证不查重", "安排", "交成品", "处理"},
			Endings:  []string{"时间紧也来得及。", "告诉我专业就能报价。", "交付以后再结算。"},
			Build: func(subject, action, ending string) string {
				return fmt.Sprintf("%s我可以%s，%s", subject, action, ending)
			},
		},
		{
			Category: "engagement_manipulation", Tags: "blog,engagement,manipulation",
			Subjects: []string{"直播间", "帖子", "评论", "评价", "店铺", "评分", "热评", "热度"},
			Actions:  []string{"安排一批", "用不同账号操作", "优化", "补点", "压下去", "见效以后再付款"},
			Endings:  []string{"真人号看不出来。", "数据很快就会好看。", "数量可以按预算调整。"},
			Build: func(subject, action, ending string) string {
				return fmt.Sprintf("这个%s可以%s，%s", subject, action, ending)
			},
		},
		{
			Category: "violence", Tags: "blog,violence,mobilization",
			Subjects: []string{"打人", "围堵", "围殴", "报复", "威胁", "恐吓", "教训"},
			Actions:  []string{"线下安排", "私聊计划", "现场集合", "找人帮忙", "发地址", "组织团队"},
			Endings:  []string{"今晚就行动。", "别在评论区留下细节。", "人多一点更容易办成。"},
			Build: func(subject, action, ending string) string {
				return fmt.Sprintf("想%s的话可以%s，%s", subject, action, ending)
			},
		},
		{
			Category: "minor_risk", Tags: "blog,minor,safety_risk",
			Subjects: []string{"未成年", "中学生", "初中", "高中", "小孩", "校园"},
			Actions:  []string{"私下交友", "深夜聊天", "交换私照", "约见", "留电话", "拉进聊天群"},
			Endings:  []string{"不要让家长知道。", "只在私下联系。", "可以付费安排。"},
			Build: func(subject, action, ending string) string {
				return fmt.Sprintf("想找%s%s，%s", subject, action, ending)
			},
		},
	}

	rows := make([]corpusRow, 0, 1494)
	appendRow := func(text, category, tags, note string) {
		rows = append(rows, corpusRow{
			ID: fmt.Sprintf("BV%04d", len(rows)+1), Text: text, Expected: "risk",
			Category: category, Tags: tags, Note: note,
		})
	}
	for _, family := range families {
		for _, subject := range family.Subjects {
			for _, action := range family.Actions {
				for _, ending := range family.Endings {
					appendRow(family.Build(subject, action, ending), family.Category, family.Tags,
						"blog comment risk scenario matrix")
				}
			}
		}
	}

	for _, subject := range []string{"资源", "资料", "账号", "课程资料", "内部资料"} {
		for _, variant := range []string{"丝聊", "司聊", "撕聊", "思聊"} {
			for _, ending := range []string{"看到就回复。", "不要公开问细节。", "价格可以商量。"} {
				appendRow(fmt.Sprintf("有%s的%s我，%s", subject, variant, ending), "spam_fraud",
					"blog,spam,homophone,predicate_variant", "homophone private-chat diversion")
			}
		}
	}

	targets := []string{"你", "你们", "您", "同学", "大家"}
	for _, target := range targets {
		for _, action := range []string{"跳楼", "割腕", "上吊", "服毒", "自杀"} {
			for _, template := range []string{
				"%s怎么不去%s",
				"%s赶紧去%s吧",
				"%s最好去%s",
				"%s有本事就去%s",
				"%s不如去%s算了",
				"%s就该去%s",
			} {
				appendRow(fmt.Sprintf(template, target, action), "harmful_value",
					"blog,harmful_value,self_harm_incitement,relation", "targeted self-harm incitement")
			}
		}
		for _, action := range []string{"去死", "别活", "不配活"} {
			for _, suffix := range []string{"吧", "得了", "算了"} {
				appendRow(target+action+suffix, "harmful_value",
					"blog,harmful_value,death_wish,relation", "directed death wish")
			}
		}
		for _, action := range []string{"卧轨", "摸高压线", "闯红灯"} {
			for _, template := range []string{
				"%s怎么不去%s",
				"%s赶紧去%s吧",
				"%s有本事就去%s",
				"%s不如去%s算了",
			} {
				appendRow(fmt.Sprintf(template, target, action), "harmful_value",
					"blog,harmful_value,dangerous_behavior,relation", "targeted dangerous behavior incitement")
			}
		}
		for _, substance := range []string{"敌敌畏", "百草枯", "农药", "鼠药"} {
			for _, ingestion := range []string{"喝", "吞", "服"} {
				for _, prefix := range []string{"去", "有本事就去"} {
					appendRow(target+prefix+ingestion+substance+"吧", "harmful_value",
						"blog,harmful_value,poison_incitement,relation", "targeted dangerous substance incitement")
				}
			}
		}
	}

	for _, target := range targets {
		for _, variant := range []string{"跳搂", "跳蒌", "跳漏", "tiao楼"} {
			appendRow(target+"怎么不去"+variant, "harmful_value",
				"blog,harmful_value,homophone,self_harm_incitement", "locally derived self-harm variant")
		}
		for _, variant := range []string{"敌敌威", "敌敌维", "didiwei"} {
			appendRow(target+"去喝"+variant+"吧", "harmful_value",
				"blog,harmful_value,homophone,poison_incitement", "locally derived dangerous substance variant")
		}
	}
	return rows
}

func mustWrite(path string, rows []corpusRow) {
	file, err := os.Create(path)
	if err != nil {
		panic(err)
	}
	writer := csv.NewWriter(file)
	writer.Comma = '\t'
	if err = writer.Write([]string{"id", "text", "expected", "category", "tags", "note"}); err != nil {
		panic(err)
	}
	for _, row := range rows {
		if err = writer.Write([]string{row.ID, row.Text, row.Expected, row.Category, row.Tags, row.Note}); err != nil {
			panic(err)
		}
	}
	writer.Flush()
	if err = writer.Error(); err != nil {
		panic(err)
	}
	if err = file.Close(); err != nil {
		panic(err)
	}
}
