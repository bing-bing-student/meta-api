package moderation

import (
	"fmt"
	"sort"
	"strings"

	appconfig "meta-api/config"
)

const (
	PolicyIssueWarning = "warning"
	PolicyIssueError   = "error"
)

// PolicyIssue 是策略静态检查结果。Path 可以直接定位到配置字段，Code 适合 CI 做稳定过滤。
type PolicyIssue struct {
	Severity string `json:"severity"`
	Code     string `json:"code"`
	Path     string `json:"path"`
	Message  string `json:"message"`
}

// LintConfig 检查不会阻止启动、但会持续增加维护成本的策略问题。
// 运行期安全错误仍由 ValidateConfig 返回；lint 用于 CI、配置评审和后续策略工具。
func LintConfig(cfg appconfig.CommentModerationConfig) []PolicyIssue {
	issues := make([]PolicyIssue, 0)
	usedConcepts := make(map[string]struct{})
	rolesByTerm := make(map[string]map[string]struct{})

	conceptIDs := make([]string, 0, len(cfg.ConceptSets))
	for id := range cfg.ConceptSets {
		conceptIDs = append(conceptIDs, id)
	}
	sort.Strings(conceptIDs)
	for _, id := range conceptIDs {
		concept := cfg.ConceptSets[id]
		path := "concept_sets." + id + ".terms"
		issues = append(issues, duplicateTermIssues(path, concept.Terms)...)
		issues = append(issues, containedTermIssues(path, concept.Terms)...)
		role := strings.ToLower(strings.TrimSpace(concept.Role))
		for _, term := range compactPolicyTerms(concept.Terms) {
			if rolesByTerm[term] == nil {
				rolesByTerm[term] = make(map[string]struct{})
			}
			rolesByTerm[term][role] = struct{}{}
		}
	}

	for index, rule := range cfg.CombinationRules {
		path := fmt.Sprintf("combination_rules[%s]", strings.TrimSpace(rule.ID))
		if strings.TrimSpace(rule.ID) == "" {
			path = fmt.Sprintf("combination_rules[%d]", index)
		}
		for _, ref := range append(append([]string(nil), rule.SubjectRefs...), rule.PredicateRefs...) {
			usedConcepts[strings.TrimSpace(ref)] = struct{}{}
		}
		issues = append(issues, duplicateTermIssues(path+".subjects", rule.Subjects)...)
		issues = append(issues, duplicateTermIssues(path+".predicates", rule.Predicates)...)
		subjects := stringSet(compactPolicyTerms(expandConceptReferences(rule.Subjects, rule.SubjectRefs, cfg.ConceptSets)))
		predicates := stringSet(compactPolicyTerms(expandConceptReferences(rule.Predicates, rule.PredicateRefs, cfg.ConceptSets)))
		for term := range subjects {
			if _, exists := predicates[term]; exists {
				issues = append(issues, PolicyIssue{Severity: PolicyIssueWarning, Code: "ambiguous_rule_role",
					Path: path, Message: fmt.Sprintf("%q 同时作为主体词和动作词", term)})
			}
		}
	}

	for _, id := range conceptIDs {
		if _, used := usedConcepts[id]; !used {
			issues = append(issues, PolicyIssue{Severity: PolicyIssueWarning, Code: "unused_concept_set",
				Path: "concept_sets." + id, Message: "概念集合未被任何组合规则引用"})
		}
	}
	for term, roles := range rolesByTerm {
		if _, subject := roles["subject"]; subject {
			if _, predicate := roles["predicate"]; predicate {
				issues = append(issues, PolicyIssue{Severity: PolicyIssueWarning, Code: "conflicting_concept_role",
					Path: "concept_sets", Message: fmt.Sprintf("%q 同时出现在主体和动作概念集合中", term)})
			}
		}
	}

	sort.SliceStable(issues, func(i, j int) bool {
		if issues[i].Path != issues[j].Path {
			return issues[i].Path < issues[j].Path
		}
		return issues[i].Code < issues[j].Code
	})
	return issues
}

// duplicateTermIssues 检查 path 对应的 values 中是否有归一化后的重复词，返回所有重复问题。
func duplicateTermIssues(path string, values []string) []PolicyIssue {
	issues := make([]PolicyIssue, 0)
	seen := make(map[string]struct{}, len(values))
	for _, term := range values {
		term = compactText(normalizeText(term))
		if term == "" {
			continue
		}
		if _, exists := seen[term]; exists {
			issues = append(issues, PolicyIssue{Severity: PolicyIssueWarning, Code: "duplicate_term",
				Path: path, Message: fmt.Sprintf("%q 重复出现", term)})
			continue
		}
		seen[term] = struct{}{}
	}
	return issues
}

// containedTermIssues 检查 path 对应的 values 中是否存在长词包含短词的模糊匹配风险，返回问题列表。
func containedTermIssues(path string, values []string) []PolicyIssue {
	terms := compactPolicyTerms(values)
	issues := make([]PolicyIssue, 0)
	for left := 0; left < len(terms); left++ {
		for right := 0; right < len(terms); right++ {
			if left == right || len([]rune(terms[left])) >= len([]rune(terms[right])) {
				continue
			}
			if strings.Contains(terms[right], terms[left]) {
				issues = append(issues, PolicyIssue{Severity: PolicyIssueWarning, Code: "contained_term",
					Path: path, Message: fmt.Sprintf("%q 被更长表达 %q 包含，请确认两者是否都需要", terms[left], terms[right])})
			}
		}
	}
	return issues
}

// stringSet 将 values 转换为去重集合，返回以原字符串为键的 map。
func stringSet(values []string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}
