package testdef

import "testing"

func TestLoadAll_ValidDirLoadsSortedByPath(t *testing.T) {
	l := NewLoader()
	tests, err := l.LoadAll("../../testdata/tests/valid")
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	// contract-assertions.yaml, full.yaml, graphql.yaml, minimal.yaml,
	// schema-file.yaml — user.schema.json is skipped (not a .yaml/.yml file).
	if len(tests) != 5 {
		t.Fatalf("expected 5 tests, got %d: %+v", len(tests), tests)
	}
	// contract-assertions.yaml < full.yaml alphabetically
	if tests[0].File[len(tests[0].File)-24:] != "contract-assertions.yaml" {
		t.Errorf("expected sorted order, first file = %s", tests[0].File)
	}
}

func TestLoadAll_InvalidDirReturnsCompileError(t *testing.T) {
	l := NewLoader()
	_, err := l.LoadAll("../../testdata/tests/invalid")
	if err == nil {
		t.Fatal("expected error loading a directory containing invalid tests")
	}
}

func TestLoadAll_MissingDirReturnsEmptyNoError(t *testing.T) {
	l := NewLoader()
	tests, err := l.LoadAll("../../testdata/does-not-exist")
	if err != nil {
		t.Fatalf("LoadAll on missing dir: %v", err)
	}
	if tests != nil {
		t.Errorf("expected nil tests for missing dir, got %v", tests)
	}
}

func TestLoadAll_StanceExampleCompiles(t *testing.T) {
	l := NewLoader()
	tests, err := l.LoadAll("../../examples/stance-graphql/.apilens/tests")
	if err != nil {
		t.Fatalf("Stance example YAML must compile: %v", err)
	}
	if len(tests) != 6 {
		t.Fatalf("expected 6 Stance example tests, got %d", len(tests))
	}
	var sawGraphQL, sawPublic, sawAuthSkip bool
	for _, tc := range tests {
		if tc.Request.GraphQL == nil || tc.Request.GraphQL.Query == "" {
			t.Errorf("%s missing request.graphql.query", tc.File)
		}
		if tc.Request.Method != "POST" {
			t.Errorf("%s method = %q, want POST", tc.File, tc.Request.Method)
		}
		if tc.Request.URL != "{{base_url}}/graphql" {
			t.Errorf("%s url = %q", tc.File, tc.Request.URL)
		}
		if tc.Assert.GraphQL == nil {
			t.Errorf("%s missing assert.graphql", tc.File)
		}
		if tc.HasTag("graphql") {
			sawGraphQL = true
		}
		if tc.HasTag("public") {
			sawPublic = true
		}
		if tc.HasTag("auth") && tc.Skip {
			sawAuthSkip = true
		}
	}
	if !sawGraphQL || !sawPublic || !sawAuthSkip {
		t.Errorf("expected graphql/public/auth-skip tags, got %+v", tests)
	}
}
