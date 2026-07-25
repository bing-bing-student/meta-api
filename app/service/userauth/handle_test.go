package userauth

import "testing"

func TestFormatUserHandleNumber(t *testing.T) {
	if got := formatUserHandleNumber(1); got != "00001" {
		t.Fatalf("expected 00001, got %q", got)
	}
	if got := formatUserHandleNumber(99999); got != "99999" {
		t.Fatalf("expected 99999, got %q", got)
	}
	if got := formatUserHandleNumber(100000); got != "100000" {
		t.Fatalf("expected 100000, got %q", got)
	}
}
