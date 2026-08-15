package testdef

import (
	"os"
	"testing"
)

func loadFixture(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading fixture %s: %v", path, err)
	}
	return data
}

func TestCompile_MinimalValidTest(t *testing.T) {
	raw := loadFixture(t, "../../testdata/tests/valid/minimal.yaml")
	tc, err := Compile(raw, "minimal.yaml")
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if tc.Name != "Health" {
		t.Errorf("Name = %q", tc.Name)
	}
	if tc.Request.Method != "GET" {
		t.Errorf("Method = %q", tc.Request.Method)
	}
	if tc.Assert.Status == nil || tc.Assert.Status.Equals == nil || *tc.Assert.Status.Equals != 200 {
		t.Errorf("Assert.Status = %+v", tc.Assert.Status)
	}
	if tc.Timeout != DefaultTimeout {
		t.Errorf("Timeout = %v, want default %v", tc.Timeout, DefaultTimeout)
	}
	if tc.Retries != DefaultRetries {
		t.Errorf("Retries = %d, want default %d", tc.Retries, DefaultRetries)
	}
}

func TestCompile_FullValidTest(t *testing.T) {
	raw := loadFixture(t, "../../testdata/tests/valid/full.yaml")
	tc, err := Compile(raw, "full.yaml")
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if len(tc.Tags) != 2 {
		t.Errorf("Tags = %v", tc.Tags)
	}
	if tc.Request.Auth == nil || tc.Request.Auth.Type != "bearer" {
		t.Errorf("Auth = %+v", tc.Request.Auth)
	}
	if len(tc.Assert.Headers) != 2 {
		t.Errorf("Assert.Headers = %+v", tc.Assert.Headers)
	}
	if len(tc.Assert.JSON) != 4 {
		t.Errorf("Assert.JSON = %+v", tc.Assert.JSON)
	}
}

func TestCompile_MissingNameFails(t *testing.T) {
	raw := loadFixture(t, "../../testdata/tests/invalid/missing-name.yaml")
	_, err := Compile(raw, "missing-name.yaml")
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestCompile_NoAssertionsFails(t *testing.T) {
	raw := loadFixture(t, "../../testdata/tests/invalid/no-assertions.yaml")
	_, err := Compile(raw, "no-assertions.yaml")
	if err == nil {
		t.Fatal("expected error for test with no assertions")
	}
}

func TestCompile_BothBodyFormsFails(t *testing.T) {
	raw := loadFixture(t, "../../testdata/tests/invalid/both-body-forms.yaml")
	_, err := Compile(raw, "both-body-forms.yaml")
	if err == nil {
		t.Fatal("expected error for both json and raw body")
	}
}

func TestCompile_BadYAMLFails(t *testing.T) {
	_, err := Compile([]byte("not: valid: yaml: at: all:"), "bad.yaml")
	if err == nil {
		t.Fatal("expected error for malformed YAML")
	}
}

func TestCompile_InvalidTimeoutFails(t *testing.T) {
	raw := []byte(`
name: Test
request:
  method: GET
  url: "{{base_url}}/x"
  timeout: not-a-duration
assert:
  status:
    equals: 200
`)
	_, err := Compile(raw, "bad-timeout.yaml")
	if err == nil {
		t.Fatal("expected error for invalid timeout")
	}
}
