package hardcover

import (
	"context"
	"fmt"
)

// SetReadProgress records listening progress on a user_book's most recent
// read record, creating one if the book has none. Hardcover creates a read
// record automatically when a book is added as Currently Reading, so the
// update path is the common one.
//
// percent is converted to seconds against the entry's Hardcover edition
// length, because Hardcover shows progress_seconds / edition length; using
// Audible's runtime instead skews the percent when the two lengths differ.
// fallbackSeconds is used when the edition has no length.
//
// Hardcover's read mutations report failures in their own "error" field
// with an HTTP 200, so that field is checked as well as GraphQL errors.
func (c *Client) SetReadProgress(ctx context.Context, userBookID int64, percent float64, fallbackSeconds int) error {
	readID, audioSeconds, err := c.latestRead(ctx, userBookID)
	if err != nil {
		return err
	}
	seconds := fallbackSeconds
	if audioSeconds > 0 {
		seconds = ProgressSecondsFor(percent, audioSeconds)
	}
	progress := map[string]any{"progress_seconds": seconds}

	if readID == 0 {
		var out struct {
			Result mutationResult `json:"insert_user_book_read"`
		}
		q := `mutation InsertUserBookRead($userBookID: Int!, $read: DatesReadInput!) {
			insert_user_book_read(user_book_id: $userBookID, user_book_read: $read) { id error }
		}`
		if err := c.do(ctx, q, map[string]any{"userBookID": userBookID, "read": progress}, &out); err != nil {
			return fmt.Errorf("insert user_book_read: %w", err)
		}
		return out.Result.err("insert user_book_read")
	}

	var out struct {
		Result mutationResult `json:"update_user_book_read"`
	}
	q := `mutation UpdateUserBookRead($id: Int!, $object: DatesReadInput!) {
		update_user_book_read(id: $id, object: $object) { id error }
	}`
	if err := c.do(ctx, q, map[string]any{"id": readID, "object": progress}, &out); err != nil {
		return fmt.Errorf("update user_book_read: %w", err)
	}
	return out.Result.err("update user_book_read")
}

// ProgressSecondsFor converts a percent complete into seconds of a book
// lengthSeconds long.
func ProgressSecondsFor(percent float64, lengthSeconds int) int {
	return int(percent / 100 * float64(lengthSeconds))
}

// latestRead returns the newest read record's id (0 if there is none) and
// the user_book's edition length in seconds (0 if unknown).
func (c *Client) latestRead(ctx context.Context, userBookID int64) (readID int64, audioSeconds int, err error) {
	var out struct {
		Reads []struct {
			ID int64 `json:"id"`
		} `json:"user_book_reads"`
		UserBooks []struct {
			Edition *struct {
				AudioSeconds *int `json:"audio_seconds"`
			} `json:"edition"`
		} `json:"user_books"`
	}
	q := `query LatestRead($userBookID: Int!) {
		user_book_reads(where: {user_book_id: {_eq: $userBookID}}, order_by: {id: desc}, limit: 1) { id }
		user_books(where: {id: {_eq: $userBookID}}) { edition { audio_seconds } }
	}`
	if err := c.do(ctx, q, map[string]any{"userBookID": userBookID}, &out); err != nil {
		return 0, 0, fmt.Errorf("find latest user_book_read: %w", err)
	}
	if len(out.Reads) > 0 {
		readID = out.Reads[0].ID
	}
	if len(out.UserBooks) > 0 && out.UserBooks[0].Edition != nil && out.UserBooks[0].Edition.AudioSeconds != nil {
		audioSeconds = *out.UserBooks[0].Edition.AudioSeconds
	}
	return readID, audioSeconds, nil
}

type mutationResult struct {
	ID    int64   `json:"id"`
	Error *string `json:"error"`
}

func (r mutationResult) err(op string) error {
	if r.Error != nil && *r.Error != "" {
		return fmt.Errorf("%s: %s", op, *r.Error)
	}
	return nil
}
