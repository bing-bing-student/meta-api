package comment

import "testing"

func TestIsValidStatus(t *testing.T) {
	for _, status := range []string{StatusPending, StatusApproved, StatusRejected} {
		if !IsValidStatus(status) {
			t.Fatalf("expected status %q to be valid", status)
		}
	}
	if IsValidStatus("deleted") {
		t.Fatal("unexpected valid status")
	}
}
