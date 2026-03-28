package service

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestRegulatoryChangeDeduplication(t *testing.T) {
	// Deduplication uses SHA-256 of URL + title
	existing := map[string]bool{
		hashContent("https://ico.org.uk/news/123", "New ICO Guidance on AI"):      true,
		hashContent("https://www.bsi.bund.de/news/456", "BSI Sicherheitshinweis"): true,
	}

	tests := []struct {
		name        string
		url         string
		title       string
		isDuplicate bool
	}{
		{"exact duplicate", "https://ico.org.uk/news/123", "New ICO Guidance on AI", true},
		{"new URL", "https://ico.org.uk/news/789", "Different Guidance", false},
		{"same title different URL", "https://ico.org.uk/news/999", "New ICO Guidance on AI", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash := hashContent(tt.url, tt.title)
			_, found := existing[hash]
			if found != tt.isDuplicate {
				t.Errorf("expected isDuplicate=%v, got %v", tt.isDuplicate, found)
			}
		})
	}
}

func hashContent(url, title string) string {
	h := sha256.New()
	h.Write([]byte(url + "|" + title))
	return hex.EncodeToString(h.Sum(nil))
}

func TestImpactLevelClassification(t *testing.T) {
	tests := []struct {
		name             string
		affectedControls int
		totalControls    int
		expectedLevel    string
	}{
		{"no controls affected", 0, 100, "none"},
		{"minor impact", 2, 100, "low"},
		{"moderate impact", 8, 100, "moderate"},
		{"significant impact", 20, 100, "significant"},
		{"critical impact", 50, 100, "critical"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			level := classifyImpactLevel(tt.affectedControls, tt.totalControls)
			if level != tt.expectedLevel {
				t.Errorf("expected %s, got %s", tt.expectedLevel, level)
			}
		})
	}
}

func classifyImpactLevel(affected, total int) string {
	if total == 0 || affected == 0 {
		return "none"
	}
	pct := float64(affected) / float64(total) * 100
	switch {
	case pct >= 30:
		return "critical"
	case pct >= 15:
		return "significant"
	case pct >= 5:
		return "moderate"
	default:
		return "low"
	}
}
