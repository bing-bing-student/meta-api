package idutil

import (
	"strings"
	"testing"
)

func TestParseID(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		fieldName string
		want      uint64
		wantErr   bool
		errSubstr string
	}{
		{name: "valid id", input: "123456789", fieldName: "articleID", want: 123456789},
		{name: "max uint64", input: "18446744073709551615", fieldName: "id", want: 18446744073709551615},
		{name: "non-numeric", input: "abc", fieldName: "articleID", wantErr: true, errSubstr: `invalid articleID "abc"`},
		{name: "empty string", input: "", fieldName: "userID", wantErr: true, errSubstr: `invalid userID ""`},
		{name: "zero is invalid", input: "0", fieldName: "articleID", wantErr: true, errSubstr: "must be positive"},
		{name: "negative", input: "-1", fieldName: "id", wantErr: true, errSubstr: `invalid id "-1"`},
		{name: "overflow uint64", input: "18446744073709551616", fieldName: "id", wantErr: true, errSubstr: `invalid id`},
		{name: "leading whitespace", input: " 1", fieldName: "id", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseID(tt.fieldName, tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseID() err = %v, wantErr = %v", err, tt.wantErr)
			}
			if err != nil && tt.errSubstr != "" && !strings.Contains(err.Error(), tt.errSubstr) {
				t.Fatalf("ParseID() err = %q, want substring %q", err.Error(), tt.errSubstr)
			}
			if !tt.wantErr && got != tt.want {
				t.Fatalf("ParseID() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestFormatID(t *testing.T) {
	tests := []struct {
		name string
		in   uint64
		want string
	}{
		{name: "zero", in: 0, want: "0"},
		{name: "small", in: 42, want: "42"},
		{name: "snowflake-like", in: 1735299123456789, want: "1735299123456789"},
		{name: "max uint64", in: 18446744073709551615, want: "18446744073709551615"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatID(tt.in); got != tt.want {
				t.Fatalf("FormatID(%d) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestParseIDFormatIDRoundTrip(t *testing.T) {
	ids := []uint64{1, 42, 1735299123456789, 18446744073709551615}
	for _, id := range ids {
		got, err := ParseID("id", FormatID(id))
		if err != nil {
			t.Fatalf("round-trip failed for %d: %v", id, err)
		}
		if got != id {
			t.Fatalf("round-trip mismatch: in=%d out=%d", id, got)
		}
	}
}
