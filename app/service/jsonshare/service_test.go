package jsonshare

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"

	"meta-api/common/guard"
)

const (
	testFingerprint = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	testTargetID    = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

type fakeGuardEngine struct {
	out *guard.Outcome
	err error
}

func (e *fakeGuardEngine) Evaluate(context.Context, *guard.RiskRequest) (*guard.Outcome, error) {
	return e.out, e.err
}

type fakeGuardStore struct {
	tokenValue string
}

func (s *fakeGuardStore) NonceTrySet(context.Context, guard.Scene, []byte, time.Duration) (bool, error) {
	return true, nil
}

func (s *fakeGuardStore) IncrCheckRate(context.Context, string, time.Duration, int64) (bool, error) {
	return false, nil
}

func (s *fakeGuardStore) DedupTrySet(context.Context, guard.Scene, string, string, time.Duration) (bool, error) {
	return true, nil
}

func (s *fakeGuardStore) TokenIssue(_ context.Context, _ guard.Scene, _ string, tokenValue string, _ time.Duration) (bool, error) {
	s.tokenValue = tokenValue
	return true, nil
}

func (s *fakeGuardStore) TokenConsume(context.Context, guard.Scene, string) (string, bool, error) {
	if s.tokenValue == "" {
		return "", false, nil
	}
	value := s.tokenValue
	s.tokenValue = ""
	return value, true, nil
}

func TestPrecheckIssuesScopedTokenClaims(t *testing.T) {
	store := &fakeGuardStore{}
	service := NewService(zap.NewNop(), &fakeGuardEngine{
		out: &guard.Outcome{
			Decision:    guard.DecisionAccept,
			Fingerprint: testFingerprint,
		},
	}, store)

	out, err := service.Precheck(context.Background(), PrecheckRequest{
		Risk: &guard.RiskRequest{
			TargetID: testTargetID,
		},
		Scope: TokenScopeCreate,
	})
	if err != nil {
		t.Fatalf("Precheck returned error: %v", err)
	}
	if out.Token == "" {
		t.Fatal("expected token")
	}

	claims, err := parseShareTokenClaims(store.tokenValue)
	if err != nil {
		t.Fatalf("parse token claims: %v", err)
	}
	if claims.Fingerprint != testFingerprint || claims.Scope != string(TokenScopeCreate) || claims.TargetID != testTargetID {
		t.Fatalf("unexpected claims: %+v", claims)
	}
}

func TestConsumeRejectsScopeOrTargetMismatch(t *testing.T) {
	store := &fakeGuardStore{}
	claims, err := encodeShareTokenClaims(shareTokenClaims{
		Fingerprint: testFingerprint,
		Scope:       string(TokenScopeCreate),
		TargetID:    testTargetID,
	})
	if err != nil {
		t.Fatalf("encode token claims: %v", err)
	}
	store.tokenValue = claims
	service := NewService(zap.NewNop(), &fakeGuardEngine{}, store)

	out, err := service.Consume(context.Background(), ConsumeRequest{
		TokenHex: "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
		Scope:    TokenScopeMine,
		TargetID: testTargetID,
	})
	if err != nil {
		t.Fatalf("Consume returned error: %v", err)
	}
	if out.HTTPStatus != 401 {
		t.Fatalf("expected unauthorized, got %d", out.HTTPStatus)
	}
}

func TestConsumeAcceptsMatchingTokenClaims(t *testing.T) {
	store := &fakeGuardStore{}
	claims, err := encodeShareTokenClaims(shareTokenClaims{
		Fingerprint: testFingerprint,
		Scope:       string(TokenScopeCreate),
		TargetID:    testTargetID,
	})
	if err != nil {
		t.Fatalf("encode token claims: %v", err)
	}
	store.tokenValue = claims
	service := NewService(zap.NewNop(), &fakeGuardEngine{}, store)

	out, err := service.Consume(context.Background(), ConsumeRequest{
		TokenHex: "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
		Scope:    TokenScopeCreate,
		TargetID: testTargetID,
	})
	if err != nil {
		t.Fatalf("Consume returned error: %v", err)
	}
	if out.Fingerprint != testFingerprint {
		t.Fatalf("expected fingerprint %q, got %q", testFingerprint, out.Fingerprint)
	}
}
