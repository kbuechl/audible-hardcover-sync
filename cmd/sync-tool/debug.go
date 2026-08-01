// cmd/sync-tool/debug.go
package main

import (
	"context"
	"fmt"
	"io"

	audiblesdk "github.com/mstrhakr/go-audible"

	"github.com/kbuechl/audible-hardcover-sync/internal/audible"
)

// TEMPORARY diagnostic — remove after debugging the Black Tongue Thief is_finished discrepancy (2026-08-01).
//
// runDebugLibrary loads Audible credentials from credentialsPath, fetches
// the full Audible library, and prints the raw IsFinished/PercentComplete
// values for every book so they can be compared against what Audible's own
// app UI shows. Read-only: it never writes to Audible or Hardcover.
func runDebugLibrary(ctx context.Context, credentialsPath string, stdout io.Writer) error {
	client := audible.New(audiblesdk.MarketplaceUS)
	if err := client.LoadCredentialsFile(credentialsPath); err != nil {
		return fmt.Errorf("load audible credentials (run 'sync-tool auth' first): %w", err)
	}

	items, err := client.FetchLibrary(ctx)
	if err != nil {
		return fmt.Errorf("fetch audible library: %w", err)
	}

	for _, item := range items {
		fmt.Fprintf(stdout, "%-12s | IsFinished=%-5t | PercentComplete=%6.2f | %s\n",
			item.ASIN, item.IsFinished, item.PercentComplete, item.Title)
	}
	return nil
}
