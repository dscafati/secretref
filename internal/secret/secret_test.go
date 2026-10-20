package secret_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/dscafati/secretref/internal/secret"
)

const canary = "hunter2-canary"

func TestSecretNeverPrints(t *testing.T) {
	s := secret.New([]byte(canary))

	outputs := map[string]string{}
	for _, verb := range []string{"%s", "%v", "%+v", "%#v", "%q", "%x"} {
		outputs[verb] = fmt.Sprintf(verb, s)
	}
	outputs["Sprint"] = fmt.Sprint(s)

	var logs bytes.Buffer
	slog.New(slog.NewTextHandler(&logs, nil)).Info("msg", "value", s)
	outputs["slog text"] = logs.String()

	logs.Reset()
	slog.New(slog.NewJSONHandler(&logs, nil)).Info("msg", "value", s)
	outputs["slog json"] = logs.String()

	type holder struct{ Value secret.Secret }
	b, err := json.Marshal(holder{Value: s})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	outputs["json"] = string(b)

	for name, out := range outputs {
		if strings.Contains(out, canary) {
			t.Errorf("%s leaked the value: %q", name, out)
		}
		if !strings.Contains(out, "[redacted]") {
			t.Errorf("%s = %q, want it to contain [redacted]", name, out)
		}
	}
}

func TestRevealReturnsCopy(t *testing.T) {
	in := []byte(canary)
	s := secret.New(in)

	in[0] = 'X'
	got := s.Reveal()
	if string(got) != canary {
		t.Fatalf("New did not copy its input: got %q", got)
	}

	got[0] = 'Y'
	if again := s.Reveal(); string(again) != canary {
		t.Fatalf("Reveal did not return a copy: got %q", again)
	}
}
