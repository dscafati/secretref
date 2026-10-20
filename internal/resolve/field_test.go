package resolve

import (
	"errors"
	"strings"
	"testing"

	"github.com/dscafati/secretref/internal/provider"
	"github.com/dscafati/secretref/internal/secret"
)

func TestField(t *testing.T) {
	const body = `{"password":"p","n":1}`
	tests := []struct {
		name, in, field, want, wantErr string
	}{
		{"string", body, "password", "p", ""},
		{"not string", body, "n", "", "field n is not a string"},
		{"missing", body, "x", "", "field x not found"},
		{"not json", "hunter2-canary", "x", "", "secret is not a JSON object"},
		{"json array", `["hunter2-canary"]`, "x", "", "secret is not a JSON object"},
		{"json null", `null`, "x", "", "secret is not a JSON object"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractField(secret.New([]byte(tt.in)), tt.field)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want %q", err, tt.wantErr)
				}
				if !errors.Is(err, provider.ErrInvalid) {
					t.Errorf("error %v does not wrap ErrInvalid", err)
				}
				if strings.Contains(err.Error(), "canary") {
					t.Errorf("error leaks secret: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if string(got.Reveal()) != tt.want {
				t.Errorf("got %q, want %q", got.Reveal(), tt.want)
			}
		})
	}
}
