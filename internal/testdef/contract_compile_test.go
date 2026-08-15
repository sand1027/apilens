package testdef

import "testing"

func TestCompile_ContractAssertionsValidTest(t *testing.T) {
	path := "../../testdata/tests/valid/contract-assertions.yaml"
	raw := loadFixture(t, path)
	tc, err := Compile(raw, path)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if len(tc.Assert.JSON) != 3 {
		t.Fatalf("expected 3 json assertions (data, data.0, data.0.email), got %+v", tc.Assert.JSON)
	}
	if tc.Assert.JSON["data"].Length == nil || *tc.Assert.JSON["data"].Length != 2 {
		t.Errorf("data.length = %+v", tc.Assert.JSON["data"].Length)
	}
	if tc.Assert.JSON["data.0"].Schema == nil {
		t.Errorf("data.0.schema should be set")
	}
	if tc.Assert.JSON["data.0.email"].Matches == nil {
		t.Errorf("data.0.email.matches should be set")
	}
}

// TestCompile_SchemaFileResolvesRelativeToTestFile exercises v7's
// schema_file, which must resolve relative to the TEST FILE'S OWN
// directory (not the process cwd or project root) — see loadSchemaFile.
func TestCompile_SchemaFileResolvesRelativeToTestFile(t *testing.T) {
	path := "../../testdata/tests/valid/schema-file.yaml"
	raw := loadFixture(t, path)
	tc, err := Compile(raw, path)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	spec, ok := tc.Assert.JSON["data"]
	if !ok {
		t.Fatalf("expected a json.data assertion, got %+v", tc.Assert.JSON)
	}
	schemaMap, ok := spec.Schema.(map[string]any)
	if !ok {
		t.Fatalf("expected loaded schema_file to decode as map[string]any, got %T", spec.Schema)
	}
	if schemaMap["type"] != "object" {
		t.Errorf("loaded schema type = %v, want \"object\"", schemaMap["type"])
	}
}

func TestCompile_BothSchemaFormsFails(t *testing.T) {
	path := "../../testdata/tests/invalid/both-schema-forms.yaml"
	raw := loadFixture(t, path)
	_, err := Compile(raw, path)
	if err == nil {
		t.Fatal("expected error when both schema and schema_file are set")
	}
}

// TestCompile_BadJSONSchemaAndBadRegexCompileFine documents that testdef
// itself does not validate schema/regex CONTENT — that is
// internal/assertions.Engine.Compile's job (docs/02-packages.md: testdef
// validates shape, assertions.Engine compiles it into executable checks).
// A malformed schema or invalid regex still compiles at the testdef layer;
// it fails later at assertions.Engine.Compile time, still before any HTTP
// call (docs/06-test-dsl.md section 12).
func TestCompile_BadJSONSchemaAndBadRegexCompileFineAtTestdefLayer(t *testing.T) {
	for _, path := range []string{
		"../../testdata/tests/invalid/bad-json-schema.yaml",
		"../../testdata/tests/invalid/bad-regex.yaml",
	} {
		raw := loadFixture(t, path)
		if _, err := Compile(raw, path); err != nil {
			t.Errorf("Compile(%s): expected testdef to accept the shape, got %v", path, err)
		}
	}
}
