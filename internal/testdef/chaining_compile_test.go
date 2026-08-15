package testdef

import "testing"

func TestCompile_ChainingValidV2SuiteCompilesWithIDAndUsesChainingFlag(t *testing.T) {
	loginPath := "../../testdata/tests/chaining/valid/01-login.yaml"
	fetchPath := "../../testdata/tests/chaining/valid/02-fetch-created.yaml"

	login, err := Compile(loadFixture(t, loginPath), loginPath)
	if err != nil {
		t.Fatalf("Compile(login): %v", err)
	}
	if login.Version != 2 {
		t.Errorf("login.Version = %d, want 2", login.Version)
	}
	if login.ID != "login" {
		t.Errorf("login.ID = %q, want %q", login.ID, "login")
	}
	if login.UsesChaining {
		t.Error("login itself does not reference any response — UsesChaining should be false")
	}

	fetch, err := Compile(loadFixture(t, fetchPath), fetchPath)
	if err != nil {
		t.Fatalf("Compile(fetch): %v", err)
	}
	if fetch.ID != "fetch-created-user" {
		t.Errorf("fetch.ID = %q", fetch.ID)
	}
	if !fetch.UsesChaining {
		t.Error("fetch references {{responses.login...}} — UsesChaining should be true")
	}
}

func TestCompile_ChainingSyntaxWithoutVersion2Fails(t *testing.T) {
	path := "../../testdata/tests/chaining/invalid-v1-chaining/uses-responses-without-v2.yaml"
	_, err := Compile(loadFixture(t, path), path)
	if err == nil {
		t.Fatal("expected an error compiling {{responses....}} syntax under version 1 (implicit or explicit)")
	}
}

func TestCompile_IDFieldWithoutVersion2Fails(t *testing.T) {
	path := "../../testdata/tests/chaining/invalid-id-without-v2/id-under-v1.yaml"
	_, err := Compile(loadFixture(t, path), path)
	if err == nil {
		t.Fatal("expected an error setting \"id\" under version 1 (implicit)")
	}
}

func TestCompile_UnsupportedVersionFails(t *testing.T) {
	path := "../../testdata/tests/chaining/invalid-unsupported-version/too-new.yaml"
	_, err := Compile(loadFixture(t, path), path)
	if err == nil {
		t.Fatal("expected an error for a version this build does not understand")
	}
}

func TestCompile_UnresolvedReferenceCompilesFine(t *testing.T) {
	// Whether "never-ran" actually produced a response is a RUNTIME
	// question the suite can't answer until it executes (docs/06-test-dsl.md
	// section 12) — testdef.Compile only validates DSL SHAPE, so this must
	// compile successfully; the failure (if any) surfaces later from
	// environment.Resolver.Interpolate during testrunner.Run.
	path := "../../testdata/tests/chaining/unresolved-reference/references-unrun-test.yaml"
	tc, err := Compile(loadFixture(t, path), path)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if !tc.UsesChaining {
		t.Error("expected UsesChaining to be true")
	}
}

func TestDocumentUsesChaining_DetectsReferenceInHeaderValue(t *testing.T) {
	doc := document{
		Request: requestDoc{
			Headers: map[string]string{"Authorization": "Bearer {{responses.login.body.token}}"},
		},
	}
	if !documentUsesChaining(doc) {
		t.Error("expected a chaining reference tucked into a header value to be detected")
	}
}

func TestDocumentUsesChaining_DetectsReferenceInJSONBodyLeaf(t *testing.T) {
	doc := document{
		Request: requestDoc{
			Body: bodyDoc{JSON: map[string]any{"token": "{{responses.login.body.data.token}}"}},
		},
	}
	if !documentUsesChaining(doc) {
		t.Error("expected a chaining reference nested in a JSON body leaf to be detected")
	}
}

func TestDocumentUsesChaining_FalseForOrdinaryVariable(t *testing.T) {
	doc := document{
		Request: requestDoc{URL: "{{base_url}}/api/users/{{user_id}}"},
	}
	if documentUsesChaining(doc) {
		t.Error("ordinary {{var}} references must not be flagged as chaining")
	}
}
