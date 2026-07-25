// internal/matcher/matcher.go
package matcher

// AudibleItem is the minimal identifying data needed to match an Audible
// library entry to a Hardcover book.
type AudibleItem struct {
	ASIN   string
	ISBN13 string
	ISBN10 string
}

// BestISBN returns the preferred ISBN to search Hardcover with (ISBN-13
// over ISBN-10), and false if the item has no usable ISBN at all — in
// which case the caller must fall back to the manual-match flow, never to
// fuzzy title/author matching.
func BestISBN(item AudibleItem) (isbn string, ok bool) {
	if item.ISBN13 != "" {
		return item.ISBN13, true
	}
	if item.ISBN10 != "" {
		return item.ISBN10, true
	}
	return "", false
}
