package moderation

import (
	"testing"

	appconfig "meta-api/config"
)

func TestCompileConfigExpandsConceptReferencesAndDeduplicates(t *testing.T) {
	cfg := appconfig.CommentModerationConfig{
		Categories: map[string]appconfig.CommentModerationCategoryConfig{
			"spam_fraud": {DefaultLevel: LevelReview},
		},
		ConceptSets: map[string]appconfig.CommentModerationConceptSetConfig{
			"academic": {Role: "subject", Terms: []string{"论文", "课程设计", "论文"}},
			"service":  {Role: "predicate", Terms: []string{"代做", "处理"}},
		},
		CombinationRules: []appconfig.CommentModerationCombinationRuleConfig{{
			ID: "academic_service", Category: "spam_fraud",
			Subjects: []string{"毕业设计"}, SubjectRefs: []string{"academic"},
			Predicates: []string{"交付"}, PredicateRefs: []string{"service"},
		}},
	}
	if err := ValidateConfig(cfg); err != nil {
		t.Fatal(err)
	}
	compiled := compileConfig(cfg)
	rule := compiled.CombinationRules[0]
	if len(rule.Subjects) != 3 || len(rule.Predicates) != 3 ||
		!relationRuleContainsTerm(rule.Subjects, "论文") || !relationRuleContainsTerm(rule.Predicates, "代做") {
		t.Fatalf("compiled rule = %+v", rule)
	}
}

func TestValidateConfigRejectsUnknownOrIncompatibleConceptReference(t *testing.T) {
	base := appconfig.CommentModerationConfig{
		ConceptSets: map[string]appconfig.CommentModerationConceptSetConfig{
			"actions": {Role: "predicate", Terms: []string{"代做"}},
		},
		CombinationRules: []appconfig.CommentModerationCombinationRuleConfig{{
			ID: "broken", Subjects: []string{"论文"}, Predicates: []string{"出售"},
			SubjectRefs: []string{"actions"},
		}},
	}
	if err := ValidateConfig(base); err == nil {
		t.Fatal("expected incompatible concept role to be rejected")
	}
	base.CombinationRules[0].SubjectRefs = []string{"missing"}
	if err := ValidateConfig(base); err == nil {
		t.Fatal("expected unknown concept reference to be rejected")
	}
}

func TestLintConfigReportsDuplicateAndUnusedConcepts(t *testing.T) {
	cfg := appconfig.CommentModerationConfig{ConceptSets: map[string]appconfig.CommentModerationConceptSetConfig{
		"unused": {Role: "subject", Terms: []string{"论文", "论文", "毕业论文"}},
	}}
	issues := LintConfig(cfg)
	codes := make(map[string]bool)
	for _, issue := range issues {
		codes[issue.Code] = true
	}
	if !codes["duplicate_term"] || !codes["contained_term"] || !codes["unused_concept_set"] {
		t.Fatalf("lint issues = %+v", issues)
	}
}
