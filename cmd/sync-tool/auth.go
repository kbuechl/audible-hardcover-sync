package main

import (
	"bufio"
	"context"
	"fmt"
	"io"

	audiblesdk "github.com/mstrhakr/go-audible"
)

// authClient is the subset of *audible.Client needed for the auth flow,
// so it can be faked in tests.
type authClient interface {
	GetAuthURL() (*audiblesdk.AuthURL, error)
	Authenticate(ctx context.Context, req audiblesdk.DeviceRegistrationRequest) error
	SaveCredentialsFile(path string) error
}

func runAuth(ctx context.Context, client authClient, credentialsPath string, stdin io.Reader, stdout io.Writer) error {
	authURL, err := client.GetAuthURL()
	if err != nil {
		return fmt.Errorf("get auth url: %w", err)
	}

	fmt.Fprintln(stdout, "Visit this URL, sign in, then paste the full redirect URL below:")
	fmt.Fprintln(stdout, authURL.URL)
	fmt.Fprint(stdout, "Redirect URL: ")

	scanner := bufio.NewScanner(stdin)
	if !scanner.Scan() {
		return fmt.Errorf("no redirect URL provided")
	}
	redirectURL := scanner.Text()

	code, err := audiblesdk.HandleAuthRedirect(redirectURL)
	if err != nil {
		return fmt.Errorf("parse redirect url: %w", err)
	}

	if err := client.Authenticate(ctx, audiblesdk.DeviceRegistrationRequest{
		AuthorizationCode: code,
		CodeVerifier:      authURL.CodeVerifier,
		DeviceSerial:      authURL.DeviceSerial,
	}); err != nil {
		return fmt.Errorf("authenticate: %w", err)
	}

	if err := client.SaveCredentialsFile(credentialsPath); err != nil {
		return fmt.Errorf("save credentials: %w", err)
	}

	fmt.Fprintf(stdout, "Authenticated. Credentials saved to %s\n", credentialsPath)
	return nil
}
