package awssm

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/credentials"

	"github.com/dscafati/secretref/internal/provider"
	"github.com/dscafati/secretref/internal/provider/providertest"
	"github.com/dscafati/secretref/internal/ref"
)

const canaryBody = "CANARY-RESPONSE-BODY"

// emulator is a minimal Secrets Manager endpoint (awsJson1.1).
type emulator struct {
	mu      sync.Mutex
	strings map[string]string
	binary  map[string][]byte
	auths   []string
	stages  []string
}

func newEmulator(t *testing.T) (*emulator, *httptest.Server) {
	t.Helper()
	e := &emulator{strings: map[string]string{}, binary: map[string][]byte{}}
	srv := httptest.NewServer(e)
	t.Cleanup(srv.Close)
	return e, srv
}

func (e *emulator) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var in struct {
		SecretID     string `json:"SecretId"`
		VersionStage string `json:"VersionStage"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	e.mu.Lock()
	defer e.mu.Unlock()
	e.auths = append(e.auths, r.Header.Get("Authorization"))
	e.stages = append(e.stages, in.VersionStage)
	w.Header().Set("Content-Type", "application/x-amz-json-1.1")
	switch {
	case strings.HasPrefix(in.SecretID, "denied"):
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"__type":"AccessDeniedException","message":"` + canaryBody + `"}`))
	case strings.HasPrefix(in.SecretID, "boom"):
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"__type":"InternalServiceError","message":"` + canaryBody + `"}`))
	}
	if v, ok := e.strings[in.SecretID]; ok {
		out, _ := json.Marshal(map[string]string{"Name": in.SecretID, "SecretString": v})
		_, _ = w.Write(out)
		return
	}
	if v, ok := e.binary[in.SecretID]; ok {
		out, _ := json.Marshal(map[string]string{"Name": in.SecretID, "SecretBinary": base64.StdEncoding.EncodeToString(v)})
		_, _ = w.Write(out)
		return
	}
	if strings.HasPrefix(in.SecretID, "denied") || strings.HasPrefix(in.SecretID, "boom") {
		return
	}
	w.WriteHeader(http.StatusBadRequest)
	_, _ = w.Write([]byte(`{"__type":"ResourceNotFoundException","message":"` + canaryBody + `"}`))
}

func testEnv(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("AWS_CONFIG_FILE", filepath.Join(dir, "config"))
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", filepath.Join(dir, "credentials"))
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	t.Setenv("AWS_ENDPOINT_URL", "")
	for _, k := range []string{
		"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN", "AWS_PROFILE", "AWS_REGION", "AWS_DEFAULT_REGION",
		"AWS_ENDPOINT_URL_SECRETS_MANAGER", "AWS_WEB_IDENTITY_TOKEN_FILE", "AWS_CONTAINER_CREDENTIALS_FULL_URI",
		"AWS_CONTAINER_CREDENTIALS_RELATIVE_URI", "AWS_CONTAINER_AUTHORIZATION_TOKEN", "AWS_SDK_LOAD_CONFIG", "AWS_ROLE_ARN",
	} {
		t.Setenv(k, "")
	}
	return dir
}

func testProvider(t *testing.T, srv *httptest.Server) *Provider {
	t.Helper()
	testEnv(t)
	return newProvider(withRetryMax(1), withEndpoint(srv.URL),
		withCredentials(credentials.NewStaticCredentialsProvider("AKIATESTSTATIC", "secret", "")))
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
	e, srv := newEmulator(t)
	p := testProvider(t, srv)
	providertest.Run(t, p, providertest.Setup{
		Put: func(t *testing.T, name, value string) ref.Ref {
			t.Helper()
			e.mu.Lock()
			e.strings["app/"+name] = value
			e.mu.Unlock()
			return mustRef(t, "aws-sm://us-east-1/app/"+name)
		},
		Missing: mustRef(t, "aws-sm://us-east-1/app/missing"),
	})
}

func TestUsesAWSCurrentStage(t *testing.T) {
	e, srv := newEmulator(t)
	e.strings["db"] = "v"
	p := testProvider(t, srv)
	if _, err := p.Resolve(context.Background(), mustRef(t, "aws-sm://us-east-1/db")); err != nil {
		t.Fatal(err)
	}
	if got := e.stages[0]; got != "AWSCURRENT" {
		t.Fatalf("VersionStage = %q, want AWSCURRENT", got)
	}
}

func TestBinarySecretIsInvalid(t *testing.T) {
	e, srv := newEmulator(t)
	e.binary["blob"] = []byte{0xff, 0x00}
	p := testProvider(t, srv)
	_, err := p.Resolve(context.Background(), mustRef(t, "aws-sm://us-east-1/blob"))
	if !errors.Is(err, provider.ErrInvalid) {
		t.Fatalf("kind = %q, want invalid", provider.Kind(err))
	}
	if !strings.Contains(err.Error(), "binary secrets are not supported") {
		t.Fatalf("error = %q", err.Error())
	}
}

func TestErrorMapping(t *testing.T) {
	_, srv := newEmulator(t)
	p := testProvider(t, srv)
	tests := []struct {
		name string
		ref  string
		want error
		code string
	}{
		{"not found", "aws-sm://us-east-1/nope", provider.ErrNotFound, "ResourceNotFoundException"},
		{"denied", "aws-sm://us-east-1/denied", provider.ErrDenied, "AccessDeniedException"},
		{"5xx", "aws-sm://us-east-1/boom", provider.ErrUnavailable, "InternalServiceError"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := p.Resolve(context.Background(), mustRef(t, tt.ref))
			if !errors.Is(err, tt.want) {
				t.Fatalf("kind = %q, want %q", provider.Kind(err), tt.want)
			}
			if !strings.Contains(err.Error(), tt.code) {
				t.Fatalf("error %q lacks code %s", err.Error(), tt.code)
			}
			if strings.Contains(err.Error(), canaryBody) {
				t.Fatalf("error text includes response body: %q", err.Error())
			}
		})
	}
}

func TestUnreachableEndpointIsUnavailable(t *testing.T) {
	testEnv(t)
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	p := newProvider(withRetryMax(1), withEndpoint(url),
		withCredentials(credentials.NewStaticCredentialsProvider("AKIATESTSTATIC", "secret", "")))
	_, err := p.Resolve(context.Background(), mustRef(t, "aws-sm://us-east-1/db"))
	if !errors.Is(err, provider.ErrUnavailable) {
		t.Fatalf("kind = %q, want unavailable", provider.Kind(err))
	}
	if !strings.Contains(err.Error(), "endpoint unreachable") {
		t.Fatalf("error = %q", err.Error())
	}
}

func TestMissingCredentialsIsDenied(t *testing.T) {
	_, srv := newEmulator(t)
	testEnv(t)
	p := newProvider(withRetryMax(1), withEndpoint(srv.URL))
	_, err := p.Resolve(context.Background(), mustRef(t, "aws-sm://us-east-1/db"))
	if !errors.Is(err, provider.ErrDenied) {
		t.Fatalf("kind = %q, want denied (err=%v)", provider.Kind(err), err)
	}
	if !strings.Contains(err.Error(), "no AWS credentials (set AWS_PROFILE, use ?profile=, or run: aws sso login)") {
		t.Fatalf("error = %q", err.Error())
	}
}

func TestMissingProfileIsInvalid(t *testing.T) {
	_, srv := newEmulator(t)
	testEnv(t)
	p := newProvider(withRetryMax(1), withEndpoint(srv.URL))
	_, err := p.Resolve(context.Background(), mustRef(t, "aws-sm://us-east-1/db?profile=dev"))
	if !errors.Is(err, provider.ErrInvalid) {
		t.Fatalf("kind = %q, want invalid (err=%v)", provider.Kind(err), err)
	}
	if !strings.Contains(err.Error(), `shared config profile "dev" not found`) {
		t.Fatalf("error = %q", err.Error())
	}
}

func TestConcurrentResolveIsRaceFree(t *testing.T) {
	e, srv := newEmulator(t)
	e.strings["db"] = "v"
	p := testProvider(t, srv)
	var wg sync.WaitGroup
	for _, region := range []string{"us-east-1", "eu-west-1", "us-east-1", "eu-west-1"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := p.Resolve(context.Background(), mustRef(t, "aws-sm://"+region+"/db")); err != nil {
				t.Errorf("Resolve failed: kind=%s", provider.Kind(err))
			}
		}()
	}
	wg.Wait()
}

func TestProfileSelectsSharedConfigProfile(t *testing.T) {
	e, srv := newEmulator(t)
	e.strings["db"] = "v"
	dir := testEnv(t)
	cfg := "[profile other]\naws_access_key_id = AKIAOTHERPROFILE\naws_secret_access_key = x\n"
	if err := os.WriteFile(filepath.Join(dir, "config"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	p := newProvider(withRetryMax(1), withEndpoint(srv.URL))

	if _, err := p.Resolve(context.Background(), mustRef(t, "aws-sm://us-east-1/db?profile=other")); err != nil {
		t.Fatalf("Resolve with profile failed: kind=%s", provider.Kind(err))
	}
	if auth := e.auths[len(e.auths)-1]; !strings.Contains(auth, "Credential=AKIAOTHERPROFILE/") {
		t.Fatalf("request not signed with the profile's key: %q", auth)
	}
}

func TestClientsAreCachedPerRegionAndProfile(t *testing.T) {
	e, srv := newEmulator(t)
	e.strings["db"] = "v"
	p := testProvider(t, srv)
	for _, raw := range []string{"aws-sm://us-east-1/db", "aws-sm://us-east-1/db", "aws-sm://eu-west-1/db"} {
		if _, err := p.Resolve(context.Background(), mustRef(t, raw)); err != nil {
			t.Fatal(err)
		}
	}
	if n := len(p.clients); n != 2 {
		t.Fatalf("cached clients = %d, want 2", n)
	}
}

func TestWrongSchemeIsInvalid(t *testing.T) {
	_, err := New().Resolve(context.Background(), mustRef(t, "keyring://svc/a"))
	if !errors.Is(err, provider.ErrInvalid) {
		t.Fatalf("kind = %q, want invalid", provider.Kind(err))
	}
}
