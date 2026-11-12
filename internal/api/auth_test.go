package api

import (
	"context"
	"testing"
)

func TestStaticTokenAuthenticator(t *testing.T) {
	authenticator, err := NewStaticTokenAuthenticator("secret-token", 7)
	if err != nil {
		t.Fatal(err)
	}

	accountID, err := authenticator.Authenticate(context.Background(), "secret-token")
	if err != nil {
		t.Fatal(err)
	}
	if accountID != 7 {
		t.Fatalf("account ID = %d, want 7", accountID)
	}

	if _, err := authenticator.Authenticate(context.Background(), "wrong-token"); err == nil {
		t.Fatal("wrong token should be rejected")
	}
}

func TestStaticTokenAuthenticatorRejectsInvalidConfiguration(t *testing.T) {
	if _, err := NewStaticTokenAuthenticator("", 1); err == nil {
		t.Fatal("empty token should be rejected")
	}
	if _, err := NewStaticTokenAuthenticator("token", 0); err == nil {
		t.Fatal("non-positive account ID should be rejected")
	}
}
