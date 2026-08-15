package cli

import "testing"

func TestDisplayVariableValue_SecretKeysStayRedacted(t *testing.T) {
	cases := []struct{ key, value string }{
		{"token", "eyJhbGciOiJIUzI1NiJ9.payload.sig"},
		{"AUTH_TOKEN", "super-secret"},
		{"password", "hunter2"},
		{"api_key", "abcd"},
	}
	for _, c := range cases {
		got := displayVariableValue(c.key, c.value)
		if got != "********" {
			t.Errorf("displayVariableValue(%q, %q) = %q, want ********", c.key, c.value, got)
		}
		if got == c.value {
			t.Errorf("secret value leaked for key %q", c.key)
		}
	}
}

func TestDisplayVariableValue_NonSecretsPassThrough(t *testing.T) {
	got := displayVariableValue("user_id", "42")
	if got != "42" {
		t.Errorf("got %q, want 42", got)
	}
}
