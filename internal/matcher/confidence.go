// internal/matcher/confidence.go
package matcher

import (
	"regexp"
	"strings"

	"github.com/adrg/strutil"
	"github.com/adrg/strutil/metrics"
)

// editionDescriptorRe matches common audiobook edition-descriptor
// parentheticals so they don't count against a title's fuzzy-match score —
// e.g. "System Collapse (Dramatized Adaptation)" should compare cleanly
// against Hardcover's plain "System Collapse".
var editionDescriptorRe = regexp.MustCompile(`(?i)\s*[\(\[][^)\]]*\b(unabridged|abridged|dramatized|adaptation|a novel|audiobook)\b[^)\]]*[\)\]]\s*`)

// NormalizeTitle lowercases, strips common audiobook edition-descriptor
// parentheticals, and collapses whitespace, for fuzzy title comparison.
func NormalizeTitle(title string) string {
	t := editionDescriptorRe.ReplaceAllString(title, " ")
	t = strings.ToLower(t)
	t = strings.Join(strings.Fields(t), " ")
	t = strings.Trim(t, " .,:;-")
	return t
}

// NormalizeAuthor lowercases and collapses whitespace for fuzzy author
// comparison. Deliberately lighter-touch than NormalizeTitle — author
// names don't carry edition-descriptor noise.
func NormalizeAuthor(author string) string {
	a := strings.ToLower(author)
	a = strings.Join(strings.Fields(a), " ")
	return strings.TrimSpace(a)
}

// CandidateInfo is the minimal identifying data BestMatch needs for one
// Hardcover search result.
type CandidateInfo struct {
	Title  string
	Author string
}

var jaroWinkler = metrics.NewJaroWinkler()

func titleAuthorScore(sourceTitle, sourceAuthor string, c CandidateInfo) (score, authorSim float64) {
	titleSim := strutil.Similarity(NormalizeTitle(sourceTitle), NormalizeTitle(c.Title), jaroWinkler)
	authorSim = strutil.Similarity(NormalizeAuthor(sourceAuthor), NormalizeAuthor(c.Author), jaroWinkler)
	score = 0.7*titleSim + 0.3*authorSim
	return score, authorSim
}

// BestMatch scores every candidate against the source title/author and
// returns the index and score of the best one. If there is exactly one
// candidate and its author similarity is >= 0.9, a +0.15 confidence bonus
// is applied (capped at 1.0) — Hardcover's own search already did real
// fuzzy title matching to produce that one result, so being the sole hit
// with a confident author match is independent evidence beyond raw title
// string distance. Returns ok=false only when candidates is empty; the
// caller is responsible for comparing the returned score against its own
// threshold — BestMatch always returns the best of what it's given.
func BestMatch(sourceTitle, sourceAuthor string, candidates []CandidateInfo) (index int, score float64, ok bool) {
	if len(candidates) == 0 {
		return 0, 0, false
	}

	bestIdx := 0
	bestScore := -1.0
	var bestAuthorSim float64
	for i, c := range candidates {
		s, authorSim := titleAuthorScore(sourceTitle, sourceAuthor, c)
		if s > bestScore {
			bestIdx = i
			bestScore = s
			bestAuthorSim = authorSim
		}
	}

	if len(candidates) == 1 && bestAuthorSim >= 0.9 {
		bestScore += 0.15
		if bestScore > 1.0 {
			bestScore = 1.0
		}
	}

	return bestIdx, bestScore, true
}
