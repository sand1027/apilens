package domain

// Environment is one `.apilens/environments/<name>.yaml` document, resolved
// (variables may still contain "${ENV}" placeholders that environment.Resolver
// expands from the process environment — see docs/06-test-dsl.md section 7).
type Environment struct {
	Name      EnvName
	BaseURL   string
	Variables map[string]string
}
