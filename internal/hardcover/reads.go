package hardcover

import (
	"context"
	"fmt"
	"time"
)

const dateLayout = "2006-01-02"

// StartRead opens a new read on a user_book for a re-listen, starting on
// startedAt with progressSeconds of progress, and returns its id.
//
// insert_user_book_read always creates a new row, so a lost response would
// otherwise produce a duplicate read on the next sync. Before inserting,
// StartRead adopts any open read (no finish date) that started on or after
// startedAt's date.
func (c *Client) StartRead(ctx context.Context, userBookID int64, startedAt time.Time, progressSeconds int) (int64, error) {
	day := startedAt.Format(dateLayout)

	var found struct {
		Reads []struct {
			ID int64 `json:"id"`
		} `json:"user_book_reads"`
	}
	q := `query OpenRead($userBookID: Int!, $since: date!) {
		user_book_reads(where: {user_book_id: {_eq: $userBookID}, finished_at: {_is_null: true}, started_at: {_gte: $since}},
			order_by: {id: desc}, limit: 1) { id }
	}`
	if err := c.do(ctx, q, map[string]any{"userBookID": userBookID, "since": day}, &found); err != nil {
		return 0, fmt.Errorf("find open user_book_read: %w", err)
	}
	if len(found.Reads) > 0 {
		return found.Reads[0].ID, nil
	}

	var out struct {
		Result mutationResult `json:"insert_user_book_read"`
	}
	m := `mutation StartRead($userBookID: Int!, $read: DatesReadInput!) {
		insert_user_book_read(user_book_id: $userBookID, user_book_read: $read) { id error }
	}`
	read := map[string]any{"started_at": day, "progress_seconds": progressSeconds}
	if err := c.do(ctx, m, map[string]any{"userBookID": userBookID, "read": read}, &out); err != nil {
		return 0, fmt.Errorf("insert user_book_read: %w", err)
	}
	if err := out.Result.err("insert user_book_read"); err != nil {
		return 0, err
	}
	return out.Result.ID, nil
}

// UpdateReadProgress sets progress_seconds on one read. Other fields of the
// read are left as they are.
func (c *Client) UpdateReadProgress(ctx context.Context, readID int64, progressSeconds int) error {
	return c.updateRead(ctx, readID, map[string]any{"progress_seconds": progressSeconds})
}

// FinishRead sets a read's finish date. Hardcover then fills its progress
// and bumps the user_book's read_count and last_read_date itself.
func (c *Client) FinishRead(ctx context.Context, readID int64, finishedAt time.Time) error {
	return c.updateRead(ctx, readID, map[string]any{"finished_at": finishedAt.Format(dateLayout)})
}

func (c *Client) updateRead(ctx context.Context, readID int64, object map[string]any) error {
	var out struct {
		Result mutationResult `json:"update_user_book_read"`
	}
	q := `mutation UpdateRead($id: Int!, $object: DatesReadInput!) {
		update_user_book_read(id: $id, object: $object) { id error }
	}`
	if err := c.do(ctx, q, map[string]any{"id": readID, "object": object}, &out); err != nil {
		return fmt.Errorf("update user_book_read: %w", err)
	}
	return out.Result.err("update user_book_read")
}
