package bootstrap

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	commentModeration "meta-api/app/service/comment/moderation"
)

func TestLoadConfigFileSetMergesModerationPolicyFiles(t *testing.T) {
	root := t.TempDir()
	policyDir := filepath.Join(root, "moderation")
	if err := os.MkdirAll(policyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestConfig(t, filepath.Join(root, "comment_moderation.manifest.yml"), `
comment_moderation:
  disabled: true
  policy_files: [moderation/first.yml, moderation/second.yml]
`)
	writeTestConfig(t, filepath.Join(policyDir, "first.yml"), `
comment_moderation:
  categories:
    spam_fraud: {name: 垃圾广告, default_level: review, feedback_enabled: true}
  semantic_rules:
    harmful_value_policy:
      disabled: true
    relation_vocabulary:
      negation_markers: [不要]
      actors: [我]
      person_targets: [你]
    risk_evaluation:
      outcomes:
        - stance: warning
          roots: [诈骗]
      judgment_predicates: [属于]
      question_markers: [是否]
  decision_engine:
    calibration:
      allow: 0.82
      block: 0.90
      script_injection_block: 0.995
      default: 0.58
      sources: {lexicon: 0.62}
  combination_rules:
    - id: first
      category: spam_fraud
      subjects: [资料]
      predicates: [私聊]
`)
	writeTestConfig(t, filepath.Join(policyDir, "second.yml"), `
comment_moderation:
  categories:
    abuse: {name: 辱骂, default_level: review, feedback_enabled: true}
  combination_rules:
    - id: second
      category: abuse
      subjects: [作者]
      predicates: [辱骂]
`)

	cfg, files, err := loadConfigFileSet([]string{filepath.Join(root, "comment_moderation.manifest.yml")})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CommentModerationConfig == nil || len(cfg.CommentModerationConfig.CombinationRules) != 2 ||
		len(cfg.CommentModerationConfig.Categories) != 2 || len(files) != 3 {
		t.Fatalf("merged policy = %+v, files = %v", cfg.CommentModerationConfig, files)
	}
}

func TestLoadRepositoryModerationPolicyBundle(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test file")
	}
	root := filepath.Dir(filepath.Dir(currentFile))
	cfg, files, err := loadConfigFileSet([]string{
		filepath.Join(root, "config/app.yml"),
		filepath.Join(root, "config/rate_limit.yml"),
		filepath.Join(root, "config/comment_moderation.manifest.yml"),
	})
	if err != nil {
		t.Fatal(err)
	}
	policy := cfg.CommentModerationConfig
	if policy == nil || len(policy.PolicyFiles) != 10 || len(policy.Categories) < 15 ||
		len(policy.ConceptSets) < 3 || len(policy.CombinationRules) < 30 ||
		len(policy.SemanticRules.RiskEvaluation.Outcomes) != 3 ||
		len(policy.DecisionEngine.Calibration.Sources) < 6 {
		t.Fatalf("incomplete moderation policy bundle: %+v (files=%v)", policy, files)
	}
	for _, issue := range commentModeration.LintConfig(*policy) {
		switch issue.Code {
		case "duplicate_term", "conflicting_concept_role", "unused_concept_set":
			t.Errorf("policy lint: %s %s: %s", issue.Code, issue.Path, issue.Message)
		}
	}
}

func TestResolveModerationPolicyFilesRejectsPathEscape(t *testing.T) {
	root := t.TempDir()
	manifest := filepath.Join(root, "comment_moderation.manifest.yml")
	if err := os.WriteFile(manifest, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveModerationPolicyFiles([]string{manifest}, []string{"../outside.yml"}); err == nil {
		t.Fatal("expected policy path escape to be rejected")
	}
}

func writeTestConfig(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
