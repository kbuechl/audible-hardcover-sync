package app

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/kbuechl/audible-hardcover-sync/internal/notify"
)

// NotifyTarget is a notification URL as shown in the web UI: masked, never
// the full URL.
type NotifyTarget struct {
	ID     int    `json:"id"`
	Masked string `json:"masked"`
}

// NotifyTargets lists the notification URLs saved in the web UI. IDs are
// positions in that list.
func (a *App) NotifyTargets() ([]NotifyTarget, error) {
	urls, err := a.store.NotifyURLs()
	if err != nil {
		return nil, err
	}
	out := make([]NotifyTarget, len(urls))
	for i, u := range urls {
		out[i] = NotifyTarget{ID: i, Masked: notify.Mask(u)}
	}
	return out, nil
}

// AddNotifyURL validates and saves a notification URL.
func (a *App) AddNotifyURL(rawURL string) error {
	rawURL = strings.TrimSpace(rawURL)
	if err := notify.Validate(rawURL); err != nil {
		return err
	}
	saved, err := a.store.NotifyURLs()
	if err != nil {
		return err
	}
	if slices.Contains(saved, rawURL) {
		return errors.New("that notification URL is already added")
	}
	return a.store.SetNotifyURLs(append(saved, rawURL))
}

// RemoveNotifyURL deletes a saved notification URL by its NotifyTargets ID.
func (a *App) RemoveNotifyURL(id int) error {
	saved, err := a.store.NotifyURLs()
	if err != nil {
		return err
	}
	if id < 0 || id >= len(saved) {
		return fmt.Errorf("no notification URL %d", id)
	}
	return a.store.SetNotifyURLs(slices.Delete(slices.Clone(saved), id, id+1))
}

// TestNotifyTarget sends a test message to a configured URL.
func (a *App) TestNotifyTarget(id int) error {
	urls, err := a.store.NotifyURLs()
	if err != nil {
		return err
	}
	if id < 0 || id >= len(urls) {
		return fmt.Errorf("no notification URL %d", id)
	}
	return notify.SendTest(urls[id])
}

// TestNotifyURL sends a test message to a URL before it is saved.
func TestNotifyURL(rawURL string) error {
	return notify.SendTest(strings.TrimSpace(rawURL))
}
