package awssm

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/smithy-go"

	"github.com/dscafati/secretref/internal/provider"
	"github.com/dscafati/secretref/internal/ref"
	"github.com/dscafati/secretref/internal/secret"
)

var _ provider.Provider = (*Provider)(nil)

const (
	versionStage   = "AWSCURRENT"
	noCredsMessage = "no AWS credentials (set AWS_PROFILE, use ?profile=, or run: aws sso login)" //nolint:gosec // user hint, not a credential
)

type clientKey struct{ region, profile string }

// Provider resolves aws-sm://<region>/<name>[?profile=P] references. It
// returns the whole secret string; JSON field extraction belongs to the
// resolver.
type Provider struct {
	endpoint string
	creds    aws.CredentialsProvider

	retryMax int

	mu      sync.Mutex
	clients map[clientKey]*clientEntry
}

// clientEntry builds one client exactly once, outside the cache lock, so
// different regions and profiles do not serialize on configuration loading.
type clientEntry struct {
	once   sync.Once
	client *secretsmanager.Client
	err    error
}

// option customizes a Provider; used by tests only.
type option func(*Provider)

// withEndpoint points every client at a fixed base endpoint.
func withEndpoint(url string) option { return func(p *Provider) { p.endpoint = url } }

// withCredentials replaces the default credential chain.
func withCredentials(c aws.CredentialsProvider) option { return func(p *Provider) { p.creds = c } }

// withRetryMax limits SDK attempts per call.
func withRetryMax(n int) option { return func(p *Provider) { p.retryMax = n } }

// New returns a Provider that uses the SDK default chain, which honors
// AWS_ENDPOINT_URL. Clients are cached per (region, profile).
func New() *Provider { return newProvider() }

func newProvider(opts ...option) *Provider {
	p := &Provider{clients: map[clientKey]*clientEntry{}}
	for _, o := range opts {
		o(p)
	}
	return p
}

// Resolve returns the current SecretString of r.
func (p *Provider) Resolve(ctx context.Context, r ref.Ref) (secret.Secret, error) {
	if r.Scheme != "aws-sm" {
		return secret.Secret{}, fmt.Errorf("aws-sm provider cannot handle scheme %q: %w", r.Scheme, provider.ErrInvalid)
	}
	client, err := p.client(ctx, r.Location, r.Opts["profile"])
	if err != nil {
		return secret.Secret{}, fmt.Errorf("%s: %s: %w", r.Raw, configReason(err), provider.ErrInvalid)
	}
	out, err := client.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		SecretId:     aws.String(r.Name),
		VersionStage: aws.String(versionStage),
	})
	if err != nil {
		return secret.Secret{}, mapError(ctx, r, err)
	}
	if out.SecretString == nil {
		return secret.Secret{}, fmt.Errorf("%s: binary secrets are not supported: %w", r.Raw, provider.ErrInvalid)
	}
	return secret.New([]byte(*out.SecretString)), nil
}

func (p *Provider) client(ctx context.Context, region, profile string) (*secretsmanager.Client, error) {
	key := clientKey{region, profile}
	p.mu.Lock()
	e, ok := p.clients[key]
	if !ok {
		e = &clientEntry{}
		p.clients[key] = e
	}
	p.mu.Unlock()

	e.once.Do(func() { e.client, e.err = p.build(ctx, region, profile) })
	if e.err != nil {
		// Do not cache failures: a fixed config file should work on the next call.
		p.mu.Lock()
		if p.clients[key] == e {
			delete(p.clients, key)
		}
		p.mu.Unlock()
	}
	return e.client, e.err
}

func (p *Provider) build(ctx context.Context, region, profile string) (*secretsmanager.Client, error) {
	loaders := []func(*config.LoadOptions) error{config.WithRegion(region)}
	if profile != "" {
		loaders = append(loaders, config.WithSharedConfigProfile(profile))
	}
	if p.creds != nil {
		loaders = append(loaders, config.WithCredentialsProvider(p.creds))
	}
	cfg, err := config.LoadDefaultConfig(ctx, loaders...)
	if err != nil {
		return nil, err
	}
	// The SDK reports a failed credential lookup as untyped wrapped text, so
	// tag it here to tell "no credentials" apart from network errors.
	cfg.Credentials = tagFailures{cfg.Credentials}
	return secretsmanager.NewFromConfig(cfg, func(o *secretsmanager.Options) {
		if p.endpoint != "" {
			o.BaseEndpoint = aws.String(p.endpoint)
		}
		if p.retryMax > 0 {
			o.RetryMaxAttempts = p.retryMax
		}
	}), nil
}

// configReason summarizes a configuration failure without echoing SDK text.
func configReason(err error) string {
	var np config.SharedConfigProfileNotExistError
	if errors.As(err, &np) {
		return fmt.Sprintf("shared config profile %q not found", np.Profile)
	}
	return "invalid AWS configuration"
}

// mapError classifies err by AWS error code. The text carries the kind and
// the code only, never the response body.
func mapError(ctx context.Context, r ref.Ref, err error) error {
	var api smithy.APIError
	if errors.As(err, &api) {
		code := api.ErrorCode()
		return fmt.Errorf("%s: AWS error %s: %w", r.Raw, code, kindForCode(code, err))
	}
	var ce *credentialsError
	if errors.As(err, &ce) && ctx.Err() == nil {
		return fmt.Errorf("%s: %s: %w", r.Raw, noCredsMessage, provider.ErrDenied)
	}
	return fmt.Errorf("%s: endpoint unreachable: %w", r.Raw, provider.ErrUnavailable)
}

func kindForCode(code string, err error) error {
	switch code {
	case "ResourceNotFoundException":
		return provider.ErrNotFound
	case "AccessDeniedException", "UnrecognizedClientException", "ExpiredTokenException",
		"InvalidClientTokenId", "ExpiredToken", "AuthFailure":
		return provider.ErrDenied
	case "DecryptionFailure", "InvalidParameterException", "InvalidRequestException":
		return provider.ErrInvalid
	}
	var re *awshttp.ResponseError
	if errors.As(err, &re) && re.HTTPStatusCode() < http.StatusInternalServerError &&
		re.HTTPStatusCode() != http.StatusTooManyRequests {
		return provider.ErrInvalid
	}
	return provider.ErrUnavailable
}

// credentialsError marks a failed credential retrieval.
type credentialsError struct{ err error }

func (e *credentialsError) Error() string { return "retrieving credentials failed" }
func (e *credentialsError) Unwrap() error { return e.err }

// tagFailures wraps retrieval failures of the inner provider in credentialsError.
type tagFailures struct{ inner aws.CredentialsProvider }

func (t tagFailures) Retrieve(ctx context.Context) (aws.Credentials, error) {
	c, err := t.inner.Retrieve(ctx)
	if err != nil {
		return aws.Credentials{}, &credentialsError{err: err}
	}
	return c, nil
}
