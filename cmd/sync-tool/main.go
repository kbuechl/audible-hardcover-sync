package main

import (
	"context"
	"fmt"
	"os"

	audiblesdk "github.com/mstrhakr/go-audible"

	"github.com/kbuechl/audible-hardcover-sync/internal/audible"
	"github.com/kbuechl/audible-hardcover-sync/internal/config"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: sync-tool <auth|sync|serve>")
		os.Exit(1)
	}

	ctx := context.Background()

	switch os.Args[1] {
	case "auth":
		credentialsPath := envOrDefault("CREDENTIALS_PATH", "/data/credentials.json")
		client := audible.New(audiblesdk.MarketplaceUS)
		if err := runAuth(ctx, client, credentialsPath, os.Stdin, os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, "auth failed:", err)
			os.Exit(1)
		}
	case "sync", "serve":
		cfg, err := config.Load()
		if err != nil {
			fmt.Fprintln(os.Stderr, "config error:", err)
			os.Exit(1)
		}
		_ = cfg // wired up in Task 11
		fmt.Fprintln(os.Stderr, os.Args[1], "not yet implemented")
		os.Exit(1)
	default:
		fmt.Fprintln(os.Stderr, "unknown subcommand:", os.Args[1])
		os.Exit(1)
	}
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
