package moderation

import "time"

const (
	LevelAllow  = "allow"
	LevelNotice = "notice"
	LevelReview = "review"
	LevelBlock  = "block"
)

const (
	SourceLexicon    = "lexicon"
	SourceStructure  = "structure"
	SourceContext    = "context"
	SourceBehavior   = "behavior"
	SourceSemantic   = "semantic"
	SourceSimilarity = "similarity"
)

const (
	defaultUserWindowSeconds        int64 = 600
	defaultIPWindowSeconds          int64 = 600
	defaultDuplicateWindowSeconds   int64 = 86400
	defaultUserReviewThreshold      int64 = 6
	defaultIPReviewThreshold        int64 = 12
	defaultDuplicateReviewThreshold int64 = 2
	defaultDuplicateBlockThreshold  int64 = 4
	defaultNearDuplicateWindow      int64 = 86400
	defaultNearDuplicateThreshold   int64 = 2
	defaultNearDuplicateDistance          = 10
	defaultNearDuplicateMinRunes          = 12
	defaultNearDuplicateMaxSamples  int64 = 100
	defaultNearDuplicateLengthDiff        = 30
	defaultFuzzyMaxDistance               = 1
	defaultFuzzyMinWordRunes              = 4
	defaultPendingScore                   = 40
	defaultRejectScore                    = 80
	behaviorTTLExtra                      = time.Minute
	base64MinLength                       = 16
	decodedURLReasonMaxLen                = 80
)

type Request struct {
	CommentID uint64
	UserID    uint64
	ArticleID uint64
	ClientIP  string
	Content   string
	Now       time.Time
}

type Result struct {
	Status   string
	Score    int
	Signals  []Signal
	Reasons  []string
	Decision string
	Trace    Trace
}

type Trace struct {
	Clauses           []ClauseTrace
	DetectorSignals   []Signal
	SuppressedSignals []Signal
	BehaviorEvaluated bool
}

type ClauseTrace struct {
	ID   int
	Text NormalizedComment
}

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

type NormalizedComment struct {
	Raw          string
	Normalized   string
	Compact      string
	Confusable   string
	PinyinFolded string
	DecodedTexts []string
}

func (n NormalizedComment) Views() []string {
	views := []string{n.Normalized, n.Compact}
	if n.Confusable != "" && n.Confusable != n.Compact {
		views = append(views, n.Confusable)
	}
	if n.PinyinFolded != "" && n.PinyinFolded != n.Compact {
		views = append(views, n.PinyinFolded)
	}
	views = append(views, n.DecodedTexts...)
	return views
}

type BehaviorState struct {
	UserCount              int64
	IPCount                int64
	DuplicateCount         int64
	NearDuplicateCount     int64
	UserEvaluated          bool
	IPEvaluated            bool
	DuplicateEvaluated     bool
	NearDuplicateEvaluated bool
}

type BehaviorKeys struct {
	User          string
	IP            string
	Duplicate     string
	NearDuplicate string
}
