package kroger

import (
	"errors"
	"strings"
	"testing"

	"golang.org/x/oauth2"
)

func TestFriendlyExchangeErr_InvalidGrant(t *testing.T) {
	src := &oauth2.RetrieveError{ErrorCode: "invalid_grant"}
	got := friendlyExchangeErr(src).Error()
	if !strings.Contains(got, "auth-url") {
		t.Errorf("invalid_grant message should point at `groceries auth-url`, got: %q", got)
	}
}

func TestFriendlyExchangeErr_InvalidClient(t *testing.T) {
	src := &oauth2.RetrieveError{ErrorCode: "invalid_client"}
	got := friendlyExchangeErr(src).Error()
	if !strings.Contains(got, "config.json") {
		t.Errorf("invalid_client message should point at config.json, got: %q", got)
	}
}

func TestFriendlyExchangeErr_UnknownPreservesCause(t *testing.T) {
	cause := errors.New("network unreachable")
	got := friendlyExchangeErr(cause)
	if !errors.Is(got, cause) {
		t.Errorf("unknown error should wrap the original (errors.Is should succeed); got %v", got)
	}
}

func TestExtractCode(t *testing.T) {
	cases := []struct {
		in, want string
		wantErr  bool
	}{
		{"raw-code", "raw-code", false},
		{"http://localhost:8080/callback?code=abc&state=s", "abc", false},
		{"code=xyz&state=s", "xyz", false},
		{"   ", "", true},
	}
	for _, tc := range cases {
		got, err := extractCode(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("extractCode(%q): expected error, got %q", tc.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("extractCode(%q): unexpected error %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("extractCode(%q): got %q, want %q", tc.in, got, tc.want)
		}
	}
}
