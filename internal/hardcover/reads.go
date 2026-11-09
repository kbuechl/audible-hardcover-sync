package hardcover

import (
	"context"
	"encoding/json"
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

// userBookReads is the user_book a mutation returns, with its read ids.
type userBookReads struct {
	Reads []struct {
		ID int64 `json:"id"`
	} `json:"user_book_reads"`
}

// ids lists the read ids; nil when the mutation returned no user_book.
func (u *userBookReads) ids() []int64 {
	if u == nil {
		return nil
	}
	ids := make([]int64, len(u.Reads))
	for i, r := range u.Reads {
		ids[i] = r.ID
	}
	return ids
}

// UpdateUserBookStatus changes an existing library entry's status, and
// optionally its rating and finish date (user_date). Like the read
// mutations, update_user_book reports failures in its own "error" field.
// It returns the entry's read ids from the response, which (checked live
// on 2026-11-08) already holds the read Hardcover adds when a book moves
// to Read or Currently Reading.
func (c *Client) UpdateUserBookStatus(ctx context.Context, userBookID int64, statusID int, rating *int, userDate *time.Time) ([]int64, error) {
	object := map[string]any{"status_id": statusID}
	if rating != nil {
		object["rating"] = *rating
	}
	if userDate != nil {
		object["user_date"] = userDate.Format(dateLayout)
	}
	var out struct {
		Result struct {
			mutationResult
			UserBook *userBookReads `json:"user_book"`
		} `json:"update_user_book"`
	}
	m := `mutation UpdateUserBookStatus($id: Int!, $object: UserBookUpdateInput!) {
		update_user_book(id: $id, object: $object) { id error user_book { user_book_reads { id } } }
	}`
	if err := c.do(ctx, m, map[string]any{"id": userBookID, "object": object}, &out); err != nil {
		return nil, fmt.Errorf("update user_book: %w", err)
	}
	if err := out.Result.err("update user_book"); err != nil {
		return nil, err
	}
	return out.Result.UserBook.ids(), nil
}

// ClearFinishDate removes the finish date from a user_book's newest read.
// Hardcover fills in today's date when a book is added as Read without
// one, which is wrong for a book whose finish date is unknown.
func (c *Client) ClearFinishDate(ctx context.Context, userBookID int64) error {
	readID, _, err := c.latestRead(ctx, userBookID)
	if err != nil || readID == 0 {
		return err
	}
	return c.updateRead(ctx, readID, map[string]any{"finished_at": nil})
}

// ReadIDs returns the ids of every read on a user_book.
func (c *Client) ReadIDs(ctx context.Context, userBookID int64) ([]int64, error) {
	var out struct {
		Reads []struct {
			ID int64 `json:"id"`
		} `json:"user_book_reads"`
	}
	q := `query ReadIDs($id: Int!) {
		user_book_reads(where: {user_book_id: {_eq: $id}}) { id }
	}`
	if err := c.do(ctx, q, map[string]any{"id": userBookID}, &out); err != nil {
		return nil, fmt.Errorf("list user_book_reads: %w", err)
	}
	ids := make([]int64, len(out.Reads))
	for i, r := range out.Reads {
		ids[i] = r.ID
	}
	return ids, nil
}

// UserBookDetails is what a library entry holds beyond its status: the
// data Remove would take with it.
type UserBookDetails struct {
	Rating    *float64
	HasReview bool
	HasNotes  bool
	ReadIDs   []int64
}

// UserBookDetails returns the rating, review and notes flags and read ids
// of one library entry, or nil when the entry no longer exists.
func (c *Client) UserBookDetails(ctx context.Context, userBookID int64) (*UserBookDetails, error) {
	var out struct {
		Books []struct {
			Rating       *float64 `json:"rating"`
			HasReview    bool     `json:"has_review"`
			PrivateNotes *string  `json:"private_notes"`
			Reads        []struct {
				ID int64 `json:"id"`
			} `json:"user_book_reads"`
		} `json:"user_books"`
	}
	q := `query UserBookDetails($id: Int!) {
		user_books(where: {id: {_eq: $id}}) { rating has_review private_notes user_book_reads { id } }
	}`
	if err := c.do(ctx, q, map[string]any{"id": userBookID}, &out); err != nil {
		return nil, fmt.Errorf("fetch user_book details: %w", err)
	}
	if len(out.Books) == 0 {
		return nil, nil
	}
	b := out.Books[0]
	d := &UserBookDetails{
		Rating:    b.Rating,
		HasReview: b.HasReview,
		HasNotes:  b.PrivateNotes != nil && *b.PrivateNotes != "",
		ReadIDs:   make([]int64, len(b.Reads)),
	}
	for i, r := range b.Reads {
		d.ReadIDs[i] = r.ID
	}
	return d, nil
}

// DeleteUserBook removes a library entry and, with it, its reads. Deleting
// an entry that is already gone succeeds.
func (c *Client) DeleteUserBook(ctx context.Context, userBookID int64) error {
	return c.deleteByID(ctx, "delete_user_book", userBookID)
}

// DeleteRead removes one read. Deleting a read that is already gone
// succeeds.
func (c *Client) DeleteRead(ctx context.Context, readID int64) error {
	return c.deleteByID(ctx, "delete_user_book_read", readID)
}

// deleteByID runs a delete mutation. The delete results have no "error"
// field: a failure surfaces as a GraphQL error, and a missing id still
// returns success. The returned id is ignored because its type varies
// (delete_user_book sends a string, delete_user_book_read a number).
func (c *Client) deleteByID(ctx context.Context, mutation string, id int64) error {
	var out map[string]json.RawMessage
	m := fmt.Sprintf(`mutation Delete($id: Int!) { %s(id: $id) { id } }`, mutation)
	if err := c.do(ctx, m, map[string]any{"id": id}, &out); err != nil {
		return fmt.Errorf("%s: %w", mutation, err)
	}
	return nil
}
