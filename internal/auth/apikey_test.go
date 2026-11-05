package auth_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"kubetask.io/kubetask/internal/auth"
)

func TestAPIKeyBindingLifecycle(t *testing.T) {
	const token, keyID = "ktk_test-key", "build-ci"
	ctx := t.Context()
	account := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{Namespace: "team-a", Name: "ci", UID: "bound-uid"},
	}
	accounts := fake.NewClientset(account).CoreV1()
	path := filepath.Join(t.TempDir(), "keys.json")
	writeBindings := func(contents string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	bindings := fmt.Sprintf(`[{"id":%q,"sha256":"%x","namespace":"team-a","serviceAccount":"ci","uid":"bound-uid"}]`, keyID, sha256.Sum256([]byte(token)))
	writeBindings(bindings)
	a := auth.NewAPIKeyAuthenticator(nil, accounts, path)
	user, err := a.Authenticate(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	if user.Username != testServiceAccount || user.UID != string(account.UID) ||
		!slices.Equal(user.Groups, []string{"system:serviceaccounts", "system:serviceaccounts:team-a", "system:authenticated"}) ||
		!slices.Equal(user.Extra[auth.APIKeyIDExtra], []string{keyID}) {
		t.Fatalf("unexpected API key identity: %#v", user)
	}
	writeBindings("[]")
	if _, err := a.Authenticate(ctx, token); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatalf("removed binding must revoke key, got %v", err)
	}
	writeBindings(bindings)
	if err := accounts.ServiceAccounts(account.Namespace).Delete(ctx, account.Name, metav1.DeleteOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Authenticate(ctx, token); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatalf("deleted account must revoke key, got %v", err)
	}
	account.UID = "replacement-uid"
	if _, err := accounts.ServiceAccounts(account.Namespace).Create(ctx, account, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Authenticate(ctx, token); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatalf("replacement account must not inherit key, got %v", err)
	}
}

func TestAPIKeyAuthenticatorKeepsTokenReview(t *testing.T) {
	tokens := newAuthenticator(t, tokenReviewFunc(func(_ context.Context, review *authenticationv1.TokenReview, _ metav1.CreateOptions) (*authenticationv1.TokenReview, error) {
		if review.Spec.Token != "existing-token" {
			t.Fatalf("unexpected TokenReview token: %q", review.Spec.Token)
		}
		return &authenticationv1.TokenReview{Status: authenticationv1.TokenReviewStatus{
			Authenticated: true, Audiences: []string{testAudience}, User: authenticationv1.UserInfo{Username: testServiceAccount},
		}}, nil
	}))
	a := auth.NewAPIKeyAuthenticator(tokens, nil, filepath.Join(t.TempDir(), "missing.json"))
	user, err := a.Authenticate(t.Context(), "existing-token")
	if err != nil || user == nil || user.Username != testServiceAccount {
		t.Fatalf("non-key Bearer token did not reach TokenReview: %v, %v", user, err)
	}
}
