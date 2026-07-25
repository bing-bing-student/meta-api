package comment

import "testing"

func TestNormalizeAdminAuthorHandle(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "empty", value: " ", want: ""},
		{name: "short numeric", value: "1", want: "00001"},
		{name: "already padded", value: "00001", want: "00001"},
		{name: "five digits", value: "99999", want: "99999"},
		{name: "six digits", value: "100000", want: "100000"},
		{name: "non numeric", value: "github-user", want: "github-user"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeAdminAuthorHandle(tt.value); got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}
