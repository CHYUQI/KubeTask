package auth

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	authenticationv1 "k8s.io/api/authentication/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
)

const (
	apiKeyPrefix = "ktk_"
	// APIKeyIDExtra identifies the configured key in audit records without exposing the key itself.
	APIKeyIDExtra = "kubetask.io/api-key-id"
)

type apiKeyBinding struct {
	ID             string `json:"id"`
	SHA256         string `json:"sha256"`
	Namespace      string `json:"namespace"`
	ServiceAccount string `json:"serviceAccount"`
	UID            string `json:"uid"`
}

// APIKeyAuthenticator checks configured API keys and delegates other Bearer tokens to TokenReview.
type APIKeyAuthenticator struct {
	tokens   *ServiceAccountAuthenticator
	accounts corev1client.ServiceAccountsGetter
	path     string
}

func NewAPIKeyAuthenticator(tokens *ServiceAccountAuthenticator, accounts corev1client.ServiceAccountsGetter, path string) *APIKeyAuthenticator {
	return &APIKeyAuthenticator{tokens: tokens, accounts: accounts, path: path}
}

func (a *APIKeyAuthenticator) Authenticate(ctx context.Context, token string) (*authenticationv1.UserInfo, error) {
	if !strings.HasPrefix(token, apiKeyPrefix) {
		return a.tokens.Authenticate(ctx, token)
	}

	// Read the projected Secret on each key request so removing a binding revokes the key after projection updates.
	contents, err := os.ReadFile(a.path)
	if err != nil {
		return nil, fmt.Errorf("%w: read API key bindings: %w", ErrAuthenticationUnavailable, err)
	}
	var bindings []apiKeyBinding
	if err := json.Unmarshal(contents, &bindings); err != nil {
		return nil, fmt.Errorf("%w: invalid API key bindings JSON", ErrAuthenticationUnavailable)
	}
	digest := sha256.Sum256([]byte(token))
	encoded := []byte(hex.EncodeToString(digest[:]))
	for _, binding := range bindings {
		if subtle.ConstantTimeCompare([]byte(binding.SHA256), encoded) == 1 {
			return a.authenticateBinding(ctx, binding)
		}
	}
	return nil, ErrUnauthenticated
}

func (a *APIKeyAuthenticator) authenticateBinding(ctx context.Context, binding apiKeyBinding) (*authenticationv1.UserInfo, error) {
	if binding.ID == "" || binding.Namespace == "" || binding.ServiceAccount == "" || binding.UID == "" {
		return nil, fmt.Errorf("%w: API key binding requires id, namespace, serviceAccount and uid", ErrAuthenticationUnavailable)
	}
	account, err := a.accounts.ServiceAccounts(binding.Namespace).Get(ctx, binding.ServiceAccount, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return nil, ErrUnauthenticated
	}
	if err != nil {
		return nil, fmt.Errorf("%w: read API key ServiceAccount: %w", ErrAuthenticationUnavailable, err)
	}
	if string(account.UID) != binding.UID {
		return nil, ErrUnauthenticated
	}
	return &authenticationv1.UserInfo{
		Username: "system:serviceaccount:" + binding.Namespace + ":" + binding.ServiceAccount,
		UID:      string(account.UID),
		Groups:   []string{"system:serviceaccounts", "system:serviceaccounts:" + binding.Namespace, "system:authenticated"},
		Extra:    map[string]authenticationv1.ExtraValue{APIKeyIDExtra: {binding.ID}},
	}, nil
}
