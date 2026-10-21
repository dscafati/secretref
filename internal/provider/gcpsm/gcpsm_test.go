package gcpsm_test

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"github.com/dscafati/secretref/internal/provider"
	"github.com/dscafati/secretref/internal/provider/gcpsm"
	"github.com/dscafati/secretref/internal/provider/providertest"
	"github.com/dscafati/secretref/internal/ref"
)

const (
	canaryBody = "CANARY-RESPONSE-BODY"
	// noADCText is the whole error for missing or rejected credentials: the
	// reason first and the kind last, without the reference.
	noADCText = "no Google Application Default Credentials (run: gcloud auth application-default login): denied"
)

// backend is a minimal Secret Manager REST emulator.
type backend struct {
	mu      sync.Mutex
	secrets map[string]string // "project/secret/version" -> value
	paths   []string
}

func (b *backend) put(project, secret, version, value string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.secrets[project+"/"+secret+"/"+version] = value
}

func (b *backend) lastPath() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.paths) == 0 {
		return ""
	}
	return b.paths[len(b.paths)-1]
}

func newBackend(t *testing.T) (*backend, *httptest.Server) {
	t.Helper()
	b := &backend{secrets: map[string]string{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b.mu.Lock()
		b.paths = append(b.paths, r.URL.Path)
		b.mu.Unlock()
		var project, secret, version string
		n, _ := scan(r.URL.Path, &project, &secret, &version)
		if r.Method != http.MethodGet || !n {
			http.NotFound(w, r)
			return
		}
		b.mu.Lock()
		v, ok := b.secrets[project+"/"+secret+"/"+version]
		b.mu.Unlock()
		if !ok {
			http.Error(w, canaryBody, http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"x","payload":{"data":"` + base64.StdEncoding.EncodeToString([]byte(v)) + `"}}`))
	}))
	t.Cleanup(srv.Close)
	return b, srv
}

// scan parses /v1/projects/{p}/secrets/{s}/versions/{v}:access.
func scan(path string, project, secret, version *string) (bool, error) {
	rest, ok := strings.CutPrefix(path, "/v1/projects/")
	if !ok {
		return false, nil
	}
	rest, ok = strings.CutSuffix(rest, ":access")
	if !ok {
		return false, nil
	}
	parts := strings.Split(rest, "/")
	if len(parts) != 5 || parts[1] != "secrets" || parts[3] != "versions" {
		return false, nil
	}
	*project, *secret, *version = parts[0], parts[2], parts[4]
	return true, nil
}

func mustRef(t *testing.T, raw string) ref.Ref {
	t.Helper()
	r, err := ref.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestContract(t *testing.T) {
	b, srv := newBackend(t)
	p := gcpsm.New(gcpsm.Options{BaseURL: srv.URL, HTTPClient: srv.Client()})
	providertest.Run(t, p, providertest.Setup{
		Put: func(t *testing.T, name, value string) ref.Ref {
			t.Helper()
			b.put("proj", name, "latest", value)
			return mustRef(t, "gcp-sm://proj/"+name)
		},
		Missing: mustRef(t, "gcp-sm://proj/missing"),
	})
}

func TestVersionSelection(t *testing.T) {
	b, srv := newBackend(t)
	b.put("proj", "db", "latest", "newest")
	b.put("proj", "db", "3", "three")
	p := gcpsm.New(gcpsm.Options{BaseURL: srv.URL, HTTPClient: srv.Client()})

	got, err := p.Resolve(context.Background(), mustRef(t, "gcp-sm://proj/db?version=3"))
	if err != nil || string(got.Reveal()) != "three" {
		t.Fatalf("version=3: err=%v", err)
	}
	if want := "/v1/projects/proj/secrets/db/versions/3:access"; b.lastPath() != want {
		t.Fatalf("path = %q, want %q", b.lastPath(), want)
	}

	got, err = p.Resolve(context.Background(), mustRef(t, "gcp-sm://proj/db"))
	if err != nil || string(got.Reveal()) != "newest" {
		t.Fatalf("default version: err=%v", err)
	}
	if want := "/v1/projects/proj/secrets/db/versions/latest:access"; b.lastPath() != want {
		t.Fatalf("path = %q, want %q", b.lastPath(), want)
	}
}

func TestErrorMapping(t *testing.T) {
	tests := []struct {
		status int
		want   error
	}{
		{http.StatusNotFound, provider.ErrNotFound},
		{http.StatusUnauthorized, provider.ErrDenied},
		{http.StatusForbidden, provider.ErrDenied},
		{http.StatusBadRequest, provider.ErrInvalid},
		{http.StatusTooManyRequests, provider.ErrUnavailable},
		{http.StatusInternalServerError, provider.ErrUnavailable},
		{http.StatusServiceUnavailable, provider.ErrUnavailable},
	}
	for _, tt := range tests {
		t.Run(http.StatusText(tt.status), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, canaryBody, tt.status)
			}))
			defer srv.Close()
			p := gcpsm.New(gcpsm.Options{BaseURL: srv.URL, HTTPClient: srv.Client()})
			_, err := p.Resolve(context.Background(), mustRef(t, "gcp-sm://proj/db"))
			if !errors.Is(err, tt.want) {
				t.Fatalf("kind = %q, want %q", provider.Kind(err), tt.want)
			}
			if strings.Contains(err.Error(), canaryBody) {
				t.Fatalf("error text includes response body: %q", err.Error())
			}
			if want := fmt.Sprintf("Secret Manager returned HTTP %d: %s", tt.status, tt.want); err.Error() != want {
				t.Fatalf("error text = %q, want %q", err.Error(), want)
			}
		})
	}
}

func TestTimeoutIsUnavailable(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		<-release
	}))
	defer srv.Close()
	defer close(release)
	client := &http.Client{Timeout: 50 * time.Millisecond}
	p := gcpsm.New(gcpsm.Options{BaseURL: srv.URL, HTTPClient: client})
	_, err := p.Resolve(context.Background(), mustRef(t, "gcp-sm://proj/db"))
	if !errors.Is(err, provider.ErrUnavailable) {
		t.Fatalf("kind = %q, want unavailable", provider.Kind(err))
	}
}

// TestContextDeadlineStaysInChain checks that an expired context is reported
// as such, so the resolver can say "timed out" instead of "request failed".
func TestContextDeadlineStaysInChain(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		<-release
		_, _ = w.Write([]byte(canaryBody))
	}))
	defer srv.Close()
	defer close(release)
	p := gcpsm.New(gcpsm.Options{BaseURL: srv.URL, HTTPClient: srv.Client()})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err := p.Resolve(ctx, mustRef(t, "gcp-sm://proj/db"))
	if !errors.Is(err, context.DeadlineExceeded) || !errors.Is(err, provider.ErrUnavailable) {
		t.Fatalf("error = %v, want it to wrap context.DeadlineExceeded and unavailable", err)
	}
	if strings.Contains(err.Error(), canaryBody) || strings.Contains(err.Error(), srv.URL) {
		t.Fatalf("error text leaks transport details: %q", err.Error())
	}
}

// TestDeadlineWhileReadingBody covers a deadline that passes after the status
// line arrived but before the body did.
func TestDeadlineWhileReadingBody(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"payload":`))
		w.(http.Flusher).Flush()
		<-release
	}))
	defer srv.Close()
	defer close(release)
	p := gcpsm.New(gcpsm.Options{BaseURL: srv.URL, HTTPClient: srv.Client()})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err := p.Resolve(ctx, mustRef(t, "gcp-sm://proj/db"))
	if !errors.Is(err, context.DeadlineExceeded) || !errors.Is(err, provider.ErrUnavailable) {
		t.Fatalf("error = %v, want it to wrap context.DeadlineExceeded and unavailable", err)
	}
}

func TestMalformedPayloadIsInvalid(t *testing.T) {
	for _, body := range []string{`not json`, `{"payload":{"data":"!!!"}}`} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(body))
		}))
		p := gcpsm.New(gcpsm.Options{BaseURL: srv.URL, HTTPClient: srv.Client()})
		_, err := p.Resolve(context.Background(), mustRef(t, "gcp-sm://proj/db"))
		srv.Close()
		if !errors.Is(err, provider.ErrInvalid) {
			t.Fatalf("body %q: kind = %q, want invalid", body, provider.Kind(err))
		}
	}
}

func TestWrongSchemeIsInvalid(t *testing.T) {
	p := gcpsm.New(gcpsm.Options{BaseURL: "http://127.0.0.1:1", HTTPClient: http.DefaultClient})
	_, err := p.Resolve(context.Background(), mustRef(t, "keyring://svc/a"))
	if !errors.Is(err, provider.ErrInvalid) {
		t.Fatalf("kind = %q, want invalid", provider.Kind(err))
	}
}

func TestMissingADCIsDenied(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "")
	t.Setenv("CLOUDSDK_CONFIG", dir)
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("APPDATA", dir)
	t.Setenv("GCE_METADATA_HOST", "")

	p := gcpsm.New(gcpsm.Options{}) // construction must not fail without credentials
	_, err := p.Resolve(context.Background(), mustRef(t, "gcp-sm://proj/db"))
	if !errors.Is(err, provider.ErrDenied) {
		t.Fatalf("kind = %q, want denied (err=%v)", provider.Kind(err), err)
	}
	if want := noADCText; err.Error() != want {
		t.Fatalf("error text = %q, want %q", err.Error(), want)
	}
}

func TestTokenRefreshFailureIsDenied(t *testing.T) {
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"` + canaryBody + `"}`))
	}))
	defer tokenSrv.Close()
	apiSrv := httptest.NewServer(http.NotFoundHandler())
	defer apiSrv.Close()

	cfg := oauth2.Config{Endpoint: oauth2.Endpoint{TokenURL: tokenSrv.URL}}
	src := cfg.TokenSource(context.Background(), &oauth2.Token{RefreshToken: "r"})
	client := oauth2.NewClient(context.Background(), src)

	p := gcpsm.New(gcpsm.Options{BaseURL: apiSrv.URL, HTTPClient: client})
	_, err := p.Resolve(context.Background(), mustRef(t, "gcp-sm://proj/db"))
	if !errors.Is(err, provider.ErrDenied) {
		t.Fatalf("kind = %q, want denied (err=%v)", provider.Kind(err), err)
	}
	if err.Error() != noADCText {
		t.Fatalf("error = %q, want %q", err.Error(), noADCText)
	}
}
