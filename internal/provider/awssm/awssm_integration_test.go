//go:build integration

package awssm

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"

	"github.com/dscafati/secretref/internal/provider/providertest"
	"github.com/dscafati/secretref/internal/ref"
)

// TestLocalStack runs the contract against a Secrets Manager emulator at
// AWS_ENDPOINT_URL (for example LocalStack).
func TestLocalStack(t *testing.T) {
	if os.Getenv("AWS_ENDPOINT_URL") == "" {
		t.Skip("AWS_ENDPOINT_URL not set; start LocalStack and set it to run this test")
	}
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
