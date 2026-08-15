package specexport

import "testing"

func TestInferSchema_EmptyBodyReturnsNil(t *testing.T) {
	if s := inferSchema(nil); s != nil {
		t.Errorf("expected nil for empty body, got %+v", s)
	}
}

func TestInferSchema_NonJSONBodyReturnsNil(t *testing.T) {
	if s := inferSchema([]byte("not json")); s != nil {
		t.Errorf("expected nil for non-JSON body, got %+v", s)
	}
}

func TestInferSchema_WholeNumberFloatBecomesInteger(t *testing.T) {
	s := inferSchema([]byte(`{"id":1}`))
	if s == nil {
		t.Fatal("expected a schema")
	}
	idSchema := s.Properties["id"].Value
	if !idSchema.Type.Is("integer") {
		t.Errorf("id type = %v, want integer", idSchema.Type)
	}
}

func TestInferSchema_FractionalNumberBecomesNumber(t *testing.T) {
	s := inferSchema([]byte(`{"price":19.99}`))
	priceSchema := s.Properties["price"].Value
	if !priceSchema.Type.Is("number") {
		t.Errorf("price type = %v, want number", priceSchema.Type)
	}
}

func TestInferSchema_ArrayInfersItemSchemaFromFirstElement(t *testing.T) {
	s := inferSchema([]byte(`{"tags":["a","b"]}`))
	tagsSchema := s.Properties["tags"].Value
	if !tagsSchema.Type.Is("array") {
		t.Fatalf("tags type = %v, want array", tagsSchema.Type)
	}
	if !tagsSchema.Items.Value.Type.Is("string") {
		t.Errorf("tags items type = %v, want string", tagsSchema.Items.Value.Type)
	}
}

func TestInferSchema_NestedObjectRecurses(t *testing.T) {
	s := inferSchema([]byte(`{"data":{"id":1,"name":"Ada"}}`))
	dataSchema := s.Properties["data"].Value
	if !dataSchema.Type.Is("object") {
		t.Fatalf("data type = %v, want object", dataSchema.Type)
	}
	if !dataSchema.Properties["name"].Value.Type.Is("string") {
		t.Errorf("data.name type mismatch")
	}
	if !dataSchema.Properties["id"].Value.Type.Is("integer") {
		t.Errorf("data.id type mismatch")
	}
}

func TestInferSchema_BooleanAndNull(t *testing.T) {
	s := inferSchema([]byte(`{"active":true,"deleted":null}`))
	if !s.Properties["active"].Value.Type.Is("boolean") {
		t.Errorf("active type mismatch")
	}
	// null has no concrete type to infer — NewSchema() is untyped, not a
	// guess at some other type.
	if s.Properties["deleted"].Value.Type != nil {
		t.Errorf("deleted type = %v, want untyped for null", s.Properties["deleted"].Value.Type)
	}
}
