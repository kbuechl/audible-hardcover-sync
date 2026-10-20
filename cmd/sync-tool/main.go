package main

import (
	"context"
	"fmt"
	"os"

	audiblesdk "github.com/mstrhakr/go-audible"

	"github.com/kbuechl/audible-hardcover-sync/internal/audible"
	"github.com/kbuechl/audible-hardcover-sync/internal/config"
)

// version is set at build time (-ldflags "-X main.version=1.2.3").
var version = "dev"

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: sync-tool <auth|sync|serve|version>")
		os.Exit(1)
	}

	ctx := context.Background()

	switch os.Args[1] {
	case "auth":
		credentialsPath := config.CredentialsPath()
		client := audible.New(audiblesdk.MarketplaceUS)
		client.SetCredentialsKey(os.Getenv("CREDENTIALS_KEY"))
		if err := runAuth(ctx, client, credentialsPath, os.Stdin, os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, "auth failed:", err)
			os.Exit(1)
		}
	case "sync":
		cfg, err := config.Load()
		if err != nil {
			fmt.Fprintln(os.Stderr, "config error:", err)
			os.Exit(1)
		}
		if _, err := runOnce(ctx, cfg); err != nil {
			fmt.Fprintln(os.Stderr, "sync failed:", err)
			os.Exit(1)
		}
	case "serve":
		cfg, err := config.Load()
		if err != nil {
			fmt.Fprintln(os.Stderr, "config error:", err)
			os.Exit(1)
		}
		if err := runServe(ctx, cfg); err != nil && err != context.Canceled && err != context.DeadlineExceeded {
			fmt.Fprintln(os.Stderr, "serve failed:", err)
			os.Exit(1)
		}
	case "version", "--version":
		fmt.Println(version)
	default:
		fmt.Fprintln(os.Stderr, "unknown subcommand:", os.Args[1])
		os.Exit(1)
	}
}
