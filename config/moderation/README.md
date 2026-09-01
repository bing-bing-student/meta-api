# 评论审核策略包维护说明

`config/comment_moderation.manifest.yml` 是唯一入口。服务按入口中的 `policy_files` 顺序加载本目录文件：映射递归合并，数组追加，标量以后加载值为准。策略包内容支持热更新；新增、删除或重排文件路径后需要重启，以便文件监听器更新订阅。

## 文件职责

- `categories.yml`：分类注册表、默认处置等级和管理员反馈选项的唯一来源。
- `concepts.yml`：跨规则复用的规范主体/动作概念。
- `lexicon.yml`：精确敏感词、受限模糊候选和结构风险表达。
- `packs/*.yml`：博客业务可能出现的领域组合规则。
- `semantics.yml`：否定、对象、引用、评价、危险行为等中文语义角色。
- `behavior.yml`：用户/IP 频率、重复和近重复窗口。
- `calibration.yml`：概率阈值、证据初始强度和审核异常时的保守处置。

## 修改原则

1. 只保存规范概念，不枚举拼音、谐音、繁体、Emoji 名称和常见错别字；这些变体由本地归一化与候选算法生成。
2. 同一词族被两条以上组合规则使用时，提取到 `concepts.yml`，通过 `subject_refs` 或 `predicate_refs` 引用。
3. 新分类必须先注册到 `categories.yml`。组合规则引用未知分类、未知概念或错误角色时，服务会拒绝加载新策略。
4. 正则表达式只放在明确声明为 pattern 的字段中。无效正则会使本次启动或热更新失败，并保留上一份有效配置。
5. 不要把完整评论样本直接写成规则；应抽象为对象、动作、结果和立场，完整样本进入黄金测试集。
6. 修改阈值或证据强度后必须运行黄金集；`calibration` 数值是相对证据强度，不代表真实风险百分比。

## 概念引用示例

```yaml
comment_moderation:
  concept_sets:
    academic_objects:
      role: subject
      terms: [论文, 课程设计, 实验报告]

  combination_rules:
    - id: academic_service
      category: spam_fraud
      subject_refs: [academic_objects]
      subjects: [毕业设计]
      predicates: [代做, 交付]
```

编译器会展开引用、完成文本归一化并按首次出现顺序去重；原始配置仍保留引用关系，便于审查词汇来源。
