package resolve

import (
	"encoding/json"
	"fmt"

	"github.com/dscafati/secretref/internal/provider"
	"github.com/dscafati/secretref/internal/secret"
)

// extractField parses s as a JSON object and returns the top-level string
// field name.
//
// SECURITY: errors never include any part of the secret. They name only the
// field, which comes from the reference. encoding/json errors are discarded
// because they can quote input bytes.
func extractField(s secret.Secret, name string) (secret.Secret, error) {
	var obj map[string]json.RawMessage
	// SECURITY: Unmarshal needs the raw bytes; its errors are discarded below.
	if err := json.Unmarshal(s.Reveal(), &obj); err != nil || obj == nil {
		return secret.Secret{}, fmt.Errorf("secret is not a JSON object: %w", provider.ErrInvalid)
	}
	raw, ok := obj[name]
	if !ok {
		return secret.Secret{}, fmt.Errorf("field %s not found: %w", name, provider.ErrInvalid)
	}
	var str string
	if err := json.Unmarshal(raw, &str); err != nil {
		return secret.Secret{}, fmt.Errorf("field %s is not a string: %w", name, provider.ErrInvalid)
	}
	return secret.New([]byte(str)), nil
}
