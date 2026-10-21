//go:build integration

package awssm

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net"
	"net/url"
	"os"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"

	"github.com/dscafati/secretref/internal/provider/providertest"
	"github.com/dscafati/secretref/internal/ref"
)

// TestMotoServer runs the contract against a Secrets Manager emulator at
// AWS_ENDPOINT_URL. `make test-integration` starts Moto in a container and sets
// it. The test refuses any endpoint that is not on a loopback address, so it
// cannot reach a real AWS account.
func TestMotoServer(t *testing.T) {
	endpoint := os.Getenv("AWS_ENDPOINT_URL")
	if endpoint == "" {
		if os.Getenv("SECRETREF_REQUIRE_AWS_EMULATOR") != "" {
			t.Fatal("AWS_ENDPOINT_URL not set but SECRETREF_REQUIRE_AWS_EMULATOR is; the emulator did not start")
		}
		t.Skip("AWS_ENDPOINT_URL not set; run `make test-integration` to start the emulator")
	}
	requireLoopback(t, endpoint)

	const region = "us-east-1"
	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		t.Fatal(err)
	}
	admin := secretsmanager.NewFromConfig(cfg)
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		t.Fatal(err)
	}
	prefix := "secretref-test-" + hex.EncodeToString(buf) + "/"

	providertest.Run(t, New(), providertest.Setup{
		Put: func(t *testing.T, name, value string) ref.Ref {
			t.Helper()
			id := prefix + name
			if _, err := admin.CreateSecret(ctx, &secretsmanager.CreateSecretInput{Name: aws.String(id), SecretString: aws.String(value)}); err != nil {
				t.Fatalf("CreateSecret failed: %v", err)
			}
			t.Cleanup(func() {
				_, _ = admin.DeleteSecret(ctx, &secretsmanager.DeleteSecretInput{
					SecretId: aws.String(id), ForceDeleteWithoutRecovery: aws.Bool(true),
				})
			})
			r, err := ref.Parse("aws-sm://" + region + "/" + id)
			if err != nil {
				t.Fatal(err)
			}
			return r
		},
		Missing: func() ref.Ref {
			r, err := ref.Parse("aws-sm://" + region + "/" + prefix + "missing")
			if err != nil {
				t.Fatal(err)
			}
			return r
		}(),
	})
}

// requireLoopback fails the test unless endpoint names localhost or a loopback IP.
func requireLoopback(t *testing.T, endpoint string) {
	t.Helper()
	u, err := url.Parse(endpoint)
	if err != nil {
		t.Fatalf("AWS_ENDPOINT_URL is not a URL: %v", err)
	}
	host := u.Hostname()
	if host == "localhost" {
		return
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return
	}
	t.Fatalf("AWS_ENDPOINT_URL host %q is not a loopback address; integration tests never call a real cloud", host)
}
