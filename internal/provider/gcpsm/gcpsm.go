package gcpsm

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"github.com/dscafati/secretref/internal/provider"
	"github.com/dscafati/secretref/internal/ref"
	"github.com/dscafati/secretref/internal/secret"
)

var _ provider.Provider = (*Provider)(nil)

const (
	defaultBaseURL = "https://secretmanager.googleapis.com"
	cloudPlatform  = "https://www.googleapis.com/auth/cloud-platform"
	noADCMessage   = "no Google Application Default Credentials (run: gcloud auth application-default login)"
	tokenTimeout   = 30 * time.Second
	maxBody        = 70 << 10 // Secret Manager caps a payload at 64 KiB.
)

// Options configures a Provider. The zero value targets Google Cloud with
// Application Default Credentials.
type Options struct {
	// BaseURL overrides https://secretmanager.googleapis.com (testing only).
	BaseURL string
	// HTTPClient overrides the ADC-authenticated client.
	HTTPClient *http.Client
}

// Provider resolves gcp-sm://<project>/<secret>[?version=N] references.
type Provider struct {
	baseURL string

	mu     sync.Mutex
	client *http.Client
}

// New returns a Provider. It never fails: credentials are looked up on the
// first Resolve.
func New(o Options) *Provider {
	base := strings.TrimRight(o.BaseURL, "/")
	if base == "" {
		base = defaultBaseURL
	}
	return &Provider{baseURL: base, client: o.HTTPClient}
}

// Resolve returns the payload of the requested secret version.
func (p *Provider) Resolve(ctx context.Context, r ref.Ref) (secret.Secret, error) {
	if r.Scheme != "gcp-sm" {
		return secret.Secret{}, fmt.Errorf("gcp-sm provider cannot handle scheme %q: %w", r.Scheme, provider.ErrInvalid)
	}
	client, err := p.httpClient(ctx)
	if err != nil {
		return secret.Secret{}, fmt.Errorf("%s: %w: %s", r.Raw, provider.ErrDenied, noADCMessage)
	}

	version := r.Opts["version"]
	if version == "" {
		version = "latest"
	}
	endpoint := fmt.Sprintf("%s/v1/projects/%s/secrets/%s/versions/%s:access",
		p.baseURL, url.PathEscape(r.Location), url.PathEscape(r.Name), url.PathEscape(version))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, http.NoBody)
	if err != nil {
		return secret.Secret{}, fmt.Errorf("%s: building request: %w", r.Raw, provider.ErrInvalid)
	}
	resp, err := client.Do(req)
	if err != nil {
		var re *oauth2.RetrieveError
		if errors.As(err, &re) {
			return secret.Secret{}, fmt.Errorf("%s: %w: %s", r.Raw, provider.ErrDenied, noADCMessage)
		}
		// The *url.Error text holds the URL only, never the response.
		return secret.Secret{}, fmt.Errorf("%s: request failed: %w", r.Raw, provider.ErrUnavailable)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxBody))
		return secret.Secret{}, statusError(r, resp.StatusCode)
	}

	var body struct {
		Payload struct {
			Data string `json:"data"`
		} `json:"payload"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBody*2)).Decode(&body); err != nil {
		return secret.Secret{}, fmt.Errorf("%s: malformed response: %w", r.Raw, provider.ErrInvalid)
	}
	data, err := base64.StdEncoding.DecodeString(body.Payload.Data)
	if err != nil {
		data, err = base64.URLEncoding.DecodeString(body.Payload.Data)
	}
	if err != nil {
		return secret.Secret{}, fmt.Errorf("%s: payload is not base64: %w", r.Raw, provider.ErrInvalid)
	}
	return secret.New(data), nil
}

// httpClient returns the configured client, creating the ADC client on first
// use. A failed lookup is not cached so a later call can succeed.
func (p *Provider) httpClient(ctx context.Context) (*http.Client, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.client != nil {
		return p.client, nil
	}
	// The client outlives ctx, so build its token source from a detached one.
	// The token source and client are cached beyond this call, so they must
	// not be tied to ctx. The HTTP client in the context bounds token fetches.
	detached := context.WithValue(context.WithoutCancel(ctx), oauth2.HTTPClient, &http.Client{Timeout: tokenTimeout})
	creds, err := google.FindDefaultCredentials(detached, cloudPlatform)
	if err != nil {
		return nil, errors.New("no application default credentials")
	}
	c := oauth2.NewClient(detached, creds.TokenSource)
	p.client = c
	return c, nil
}

func statusError(r ref.Ref, status int) error {
	var kind error
	switch status {
	case http.StatusNotFound:
		kind = provider.ErrNotFound
	case http.StatusUnauthorized, http.StatusForbidden:
		kind = provider.ErrDenied
	case http.StatusBadRequest:
		kind = provider.ErrInvalid
	default:
		kind = provider.ErrUnavailable
	}
	return fmt.Errorf("%s: Secret Manager returned HTTP %d: %w", r.Raw, status, kind)
}
