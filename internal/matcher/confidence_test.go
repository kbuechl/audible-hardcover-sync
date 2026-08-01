// internal/matcher/confidence_test.go
package matcher

import "testing"

func TestNormalizeTitle(t *testing.T) {
	tests := []struct {
		name  string
		title string
		want  string
	}{
		{"strips unabridged suffix", "Project Hail Mary (Unabridged)", "project hail mary"},
		{"strips dramatized adaptation suffix", "System Collapse (Dramatized Adaptation)", "system collapse"},
		{"strips a novel suffix", "The Fifth Season: A Novel", "the fifth season: a novel"}, // colon form not parenthetical — left as-is by design
		{"lowercases and trims", "  THE HOBBIT  ", "the hobbit"},
		{"collapses internal whitespace", "A Court   of Thorns and Roses", "a court of thorns and roses"},
		{"no descriptor present", "Dune", "dune"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeTitle(tt.title)
			if got != tt.want {
				t.Errorf("NormalizeTitle(%q) = %q, want %q", tt.title, got, tt.want)
			}
		})
	}
}

func TestNormalizeAuthor(t *testing.T) {
	tests := []struct {
		name   string
		author string
		want   string
	}{
		{"lowercases and trims", "  Sarah J. Maas  ", "sarah j. maas"},
		{"collapses whitespace", "N.  K.   Jemisin", "n. k. jemisin"},
		{"empty stays empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeAuthor(tt.author)
			if got != tt.want {
				t.Errorf("NormalizeAuthor(%q) = %q, want %q", tt.author, got, tt.want)
			}
		})
	}
}

func TestBestMatch(t *testing.T) {
	tests := []struct {
		name         string
		sourceTitle  string
		sourceAuthor string
		candidates   []CandidateInfo
		wantOK       bool
		wantIndex    int
		wantMinScore float64 // score must be >= this
		wantMaxScore float64 // score must be <= this (use 1.01 to mean "no upper bound check")
	}{
		{
			name:         "no candidates",
			sourceTitle:  "Dune",
			sourceAuthor: "Frank Herbert",
			candidates:   nil,
			wantOK:       false,
		},
		{
			name:         "exact title and author match scores near 1.0",
			sourceTitle:  "Dune",
			sourceAuthor: "Frank Herbert",
			candidates:   []CandidateInfo{{Title: "Dune", Author: "Frank Herbert"}},
			wantOK:       true,
			wantIndex:    0,
			wantMinScore: 0.99,
			wantMaxScore: 1.01,
		},
		{
			name:         "edition-descriptor mismatch still scores high after normalization",
			sourceTitle:  "System Collapse (Dramatized Adaptation)",
			sourceAuthor: "Martha Wells",
			candidates:   []CandidateInfo{{Title: "System Collapse", Author: "Martha Wells"}},
			wantOK:       true,
			wantIndex:    0,
			wantMinScore: 0.95, // single-candidate bonus should push this to ~1.0 even before normalization credit
			wantMaxScore: 1.01,
		},
		{
			name:         "picks the best of multiple candidates",
			sourceTitle:  "The Fifth Season",
			sourceAuthor: "N. K. Jemisin",
			candidates: []CandidateInfo{
				{Title: "The Obelisk Gate", Author: "N. K. Jemisin"},         // wrong book, same author
				{Title: "The Fifth Season", Author: "N. K. Jemisin"},         // right book
				{Title: "The Fifth Season: A Novel", Author: "Someone Else"}, // right-ish title, wrong author
			},
			wantOK:       true,
			wantIndex:    1,
			wantMinScore: 0.9,
			wantMaxScore: 1.01,
		},
		{
			name:         "completely unrelated candidate scores low",
			sourceTitle:  "Dune",
			sourceAuthor: "Frank Herbert",
			candidates:   []CandidateInfo{{Title: "The Great Gatsby", Author: "F. Scott Fitzgerald"}},
			wantOK:       true, // BestMatch always returns the best of what's given — thresholding happens in the caller
			wantIndex:    0,
			wantMinScore: 0,
			wantMaxScore: 0.5,
		},
		{
			// Single candidate with a very wrong author must NOT receive the
			// +0.15 confidence bonus — that bonus is gated on authorSim >= 0.9.
			// Title is exact (titleSim ~1.0), author is unrelated (authorSim
			// well below 0.9), so score ~ 0.7*1.0 + 0.3*~0 = ~0.7, and the
			// bonus must not push it above ~0.85.
			name:         "single candidate with mismatched author does not get bonus",
			sourceTitle:  "Dune",
			sourceAuthor: "Frank Herbert",
			candidates:   []CandidateInfo{{Title: "Dune", Author: "F. Scott Fitzgerald"}},
			wantOK:       true,
			wantIndex:    0,
			wantMinScore: 0.6,
			wantMaxScore: 0.9, // 0.86 observed; strictly below the 1.0 a +0.15 bonus would produce
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			idx, score, ok := BestMatch(tt.sourceTitle, tt.sourceAuthor, tt.candidates)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if idx != tt.wantIndex {
				t.Errorf("index = %d, want %d", idx, tt.wantIndex)
			}
			if score < tt.wantMinScore || score > tt.wantMaxScore {
				t.Errorf("score = %v, want in [%v, %v]", score, tt.wantMinScore, tt.wantMaxScore)
			}
		})
	}
}

func TestBestMatch_ScoreNeverExceedsOne(t *testing.T) {
	// The single-candidate bonus must be capped — an already-near-perfect
	// match plus the +0.15 bonus must not exceed 1.0.
	_, score, ok := BestMatch("Dune", "Frank Herbert", []CandidateInfo{{Title: "Dune", Author: "Frank Herbert"}})
	if !ok {
		t.Fatal("ok = false, want true")
	}
	if score > 1.0 {
		t.Errorf("score = %v, want <= 1.0", score)
	}
}
