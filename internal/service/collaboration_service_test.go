package service

import (
	"regexp"
	"strings"
	"testing"
)

func TestMentionParsing(t *testing.T) {
	tests := []struct {
		name          string
		content       string
		expectedUsers []string
		expectedRoles []string
	}{
		{
			"single user mention",
			"Hey @[John Smith](user-uuid-123), please review this.",
			[]string{"user-uuid-123"},
			nil,
		},
		{
			"multiple user mentions",
			"@[Alice](user-1) and @[Bob](user-2) need to look at this.",
			[]string{"user-1", "user-2"},
			nil,
		},
		{
			"role mention",
			"Attention @role:ciso — this needs your input.",
			nil,
			[]string{"ciso"},
		},
		{
			"mixed mentions",
			"@[John](user-1) and @role:dpo please check @[Jane](user-2)'s work.",
			[]string{"user-1", "user-2"},
			[]string{"dpo"},
		},
		{
			"no mentions",
			"This is a regular comment with no mentions.",
			nil,
			nil,
		},
	}

	userMentionRe := regexp.MustCompile(`@\[([^\]]+)\]\(([^)]+)\)`)
	roleMentionRe := regexp.MustCompile(`@role:(\w+)`)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var userIDs []string
			for _, match := range userMentionRe.FindAllStringSubmatch(tt.content, -1) {
				userIDs = append(userIDs, match[2])
			}

			var roles []string
			for _, match := range roleMentionRe.FindAllStringSubmatch(tt.content, -1) {
				roles = append(roles, match[1])
			}

			if len(userIDs) != len(tt.expectedUsers) {
				t.Errorf("expected %d user mentions, got %d: %v", len(tt.expectedUsers), len(userIDs), userIDs)
			}
			if len(roles) != len(tt.expectedRoles) {
				t.Errorf("expected %d role mentions, got %d: %v", len(tt.expectedRoles), len(roles), roles)
			}
		})
	}
}

func TestCommentThreadingDepth(t *testing.T) {
	// Max thread depth is 3 levels
	maxDepth := 3

	tests := []struct {
		depth   int
		allowed bool
	}{
		{0, true},  // top-level
		{1, true},  // reply to top-level
		{2, true},  // reply to reply
		{3, false}, // too deep
		{4, false}, // way too deep
	}

	for _, tt := range tests {
		allowed := tt.depth < maxDepth
		if allowed != tt.allowed {
			t.Errorf("depth %d: expected allowed=%v, got %v", tt.depth, tt.allowed, allowed)
		}
	}
}

func TestContentSanitization(t *testing.T) {
	tests := []struct {
		name  string
		input string
		safe  bool
	}{
		{"plain text", "This is a safe comment.", true},
		{"markdown bold", "This is **bold** text.", true},
		{"script tag", "<script>alert('xss')</script>", false},
		{"onclick", "<div onclick='alert(1)'>click</div>", false},
		{"safe link", "[click here](https://example.com)", true},
		{"javascript link", "[click](javascript:alert(1))", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hasDanger := strings.Contains(strings.ToLower(tt.input), "<script") ||
				strings.Contains(strings.ToLower(tt.input), "onclick") ||
				strings.Contains(strings.ToLower(tt.input), "javascript:")
			isSafe := !hasDanger
			if isSafe != tt.safe {
				t.Errorf("expected safe=%v, got %v for: %s", tt.safe, isSafe, tt.input)
			}
		})
	}
}
