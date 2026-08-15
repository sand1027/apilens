// Package openapi implements the OpenAPI 3 / Swagger 2 discovery provider
// (docs/07-discovery.md section 3, ADR-008: parse via kin-openapi rather
// than a hand-rolled parser).
package openapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

	"github.com/getkin/kin-openapi/openapi2"
	"github.com/getkin/kin-openapi/openapi2conv"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/sandeepv/apilens/internal/discovery"
	"github.com/sandeepv/apilens/internal/domain"
	"gopkg.in/yaml.v3"
)

// wellKnownNames are checked in the project root and .apilens/api/ before
// falling back to a limited walk (docs/07-discovery.md section 3
// "Location strategy").
var wellKnownNames = []string{
	"openapi.yaml", "openapi.yml", "openapi.json",
	"swagger.yaml", "swagger.yml", "swagger.json",
}

// walkIgnoreDirs are skipped during the depth-limited walk
// (docs/07-discovery.md section 3, docs/11-risks-and-gaps.md R12).
var walkIgnoreDirs = map[string]bool{
	"node_modules": true, "vendor": true, ".git": true,
	"dist": true, "build": true, ".apilens": true,
}

// maxWalkDepth caps the fallback directory walk so a large monorepo isn't
// fully scanned by default (docs/07-discovery.md section 3, R12).
const maxWalkDepth = 4

// Provider implements discovery.Provider for OpenAPI 3 / Swagger 2 specs.
type Provider struct {
	// ConfigPaths mirrors discovery.openapi.paths from config.yaml
	// (docs/03-plugins.md section 9): explicit spec file paths checked
	// before well-known filenames.
	ConfigPaths []string
}

// New builds an OpenAPI provider. configPaths comes from
// discovery.openapi.paths in config.yaml.
func New(configPaths []string) *Provider {
	return &Provider{ConfigPaths: configPaths}
}

func (p *Provider) Name() string { return "openapi" }

// Detect is cheap by design (docs/03-plugins.md section 4: "Detect is
// cheap. No network."): it just checks whether any candidate spec file
// exists, without parsing it.
func (p *Provider) Detect(ctx context.Context, root fs.FS) (bool, error) {
	candidates, err := p.candidateFiles(root)
	if err != nil {
		return false, err
	}
	return len(candidates) > 0, nil
}

// Discover parses every candidate spec file found via ConfigPaths,
// well-known filenames, or a depth-limited walk (or opts.Paths if the
// caller forced explicit paths, i.e. --path). A broken spec is skipped
// rather than aborting the whole discovery run (docs/11-risks-and-gaps.md
// R1), UNLESS it was an explicitly requested --path, in which case the
// error is propagated.
func (p *Provider) Discover(ctx context.Context, root fs.FS, opts discovery.Options) ([]domain.Endpoint, error) {
	explicitPaths := opts.Paths
	var files []string
	if len(explicitPaths) > 0 {
		files = explicitPaths
	} else {
		var err error
		files, err = p.candidateFiles(root)
		if err != nil {
			return nil, err
		}
	}

	var endpoints []domain.Endpoint
	for _, f := range files {
		found, err := parseFile(root, f)
		if err != nil {
			if len(explicitPaths) > 0 {
				// --path pointed directly at a broken file: propagate.
				return nil, fmt.Errorf("parsing %s: %w", f, err)
			}
			// Discovered via well-known name / walk: skip and continue
			// (docs/11-risks-and-gaps.md R1).
			continue
		}
		endpoints = append(endpoints, found...)
	}
	return endpoints, nil
}

// candidateFiles implements the "Location strategy" from
// docs/07-discovery.md section 3: config paths, then well-known names in
// root and .apilens/api/, then a depth-4 walk with ignores.
func (p *Provider) candidateFiles(root fs.FS) ([]string, error) {
	var found []string
	seen := map[string]bool{}

	add := func(name string) {
		if !seen[name] {
			seen[name] = true
			found = append(found, name)
		}
	}

	for _, cp := range p.ConfigPaths {
		if fileExists(root, cp) {
			add(cp)
		}
	}
	if len(found) > 0 {
		return found, nil
	}

	for _, name := range wellKnownNames {
		if fileExists(root, name) {
			add(name)
		}
		apiPath := path.Join(".apilens", "api", name)
		if fileExists(root, apiPath) {
			add(apiPath)
		}
	}
	if len(found) > 0 {
		return found, nil
	}

	walked, err := walkForSpecs(root)
	if err != nil {
		return nil, err
	}
	for _, w := range walked {
		add(w)
	}
	return found, nil
}

func fileExists(root fs.FS, name string) bool {
	info, err := fs.Stat(root, name)
	return err == nil && !info.IsDir()
}

func walkForSpecs(root fs.FS) ([]string, error) {
	var results []string
	err := fs.WalkDir(root, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // ignore unreadable entries, don't abort the walk
		}
		if p == "." {
			return nil
		}
		depth := strings.Count(p, "/") + 1
		if d.IsDir() {
			if walkIgnoreDirs[d.Name()] || depth > maxWalkDepth {
				return fs.SkipDir
			}
			return nil
		}
		if depth > maxWalkDepth {
			return nil
		}
		name := d.Name()
		for _, wk := range wellKnownNames {
			if name == wk {
				results = append(results, p)
				break
			}
		}
		return nil
	})
	return results, err
}

// parseFile loads a single spec file, auto-detecting OpenAPI 3 vs Swagger 2
// by trying OpenAPI 3 first and falling back to Swagger 2 conversion
// (ADR-008).
func parseFile(root fs.FS, name string) ([]domain.Endpoint, error) {
	data, err := fs.ReadFile(root, name)
	if err != nil {
		return nil, err
	}

	if doc, err := loadOpenAPI3(data); err == nil {
		return endpointsFromV3(doc), nil
	}

	doc2, err := loadSwagger2(data)
	if err != nil {
		return nil, fmt.Errorf("not a valid OpenAPI 3 or Swagger 2 document: %w", err)
	}
	doc3, err := openapi2conv.ToV3(doc2)
	if err != nil {
		return nil, fmt.Errorf("converting Swagger 2 to OpenAPI 3: %w", err)
	}
	return endpointsFromV3(doc3), nil
}

func loadOpenAPI3(data []byte) (*openapi3.T, error) {
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = false
	doc, err := loader.LoadFromData(data)
	if err != nil {
		return nil, err
	}
	if doc.OpenAPI == "" {
		return nil, fmt.Errorf("missing openapi version field")
	}
	// A minimal Validate catches structurally broken specs
	// (docs/11-risks-and-gaps.md R1) without being overly strict about
	// every optional field.
	if err := doc.Validate(loader.Context); err != nil {
		return nil, err
	}
	return doc, nil
}

// loadSwagger2 parses a Swagger 2 document. openapi2.T only implements
// UnmarshalJSON (not YAML), so YAML input is normalized to JSON first —
// unmarshaling YAML directly into it silently drops/mis-decodes fields
// like parameter "type" and produces an incomplete document.
func loadSwagger2(data []byte) (*openapi2.T, error) {
	var generic any
	if err := yaml.Unmarshal(data, &generic); err != nil {
		return nil, err
	}
	jsonBytes, err := json.Marshal(generic)
	if err != nil {
		return nil, err
	}
	var doc openapi2.T
	if err := json.Unmarshal(jsonBytes, &doc); err != nil {
		return nil, err
	}
	if doc.Swagger == "" {
		return nil, fmt.Errorf("missing swagger version field")
	}
	return &doc, nil
}

// endpointsFromV3 implements the mapping table in docs/07-discovery.md
// section 3.
func endpointsFromV3(doc *openapi3.T) []domain.Endpoint {
	if doc.Paths == nil {
		return nil
	}
	var out []domain.Endpoint
	paths := doc.Paths.Map()
	pathKeys := make([]string, 0, len(paths))
	for p := range paths {
		pathKeys = append(pathKeys, p)
	}
	sort.Strings(pathKeys)

	methodOrder := []struct {
		name string
		get  func(*openapi3.PathItem) *openapi3.Operation
	}{
		{"GET", func(pi *openapi3.PathItem) *openapi3.Operation { return pi.Get }},
		{"POST", func(pi *openapi3.PathItem) *openapi3.Operation { return pi.Post }},
		{"PUT", func(pi *openapi3.PathItem) *openapi3.Operation { return pi.Put }},
		{"PATCH", func(pi *openapi3.PathItem) *openapi3.Operation { return pi.Patch }},
		{"DELETE", func(pi *openapi3.PathItem) *openapi3.Operation { return pi.Delete }},
		{"HEAD", func(pi *openapi3.PathItem) *openapi3.Operation { return pi.Head }},
		{"OPTIONS", func(pi *openapi3.PathItem) *openapi3.Operation { return pi.Options }},
	}

	for _, rawPath := range pathKeys {
		item := paths[rawPath]
		for _, m := range methodOrder {
			op := m.get(item)
			if op == nil {
				continue
			}
			out = append(out, domain.Endpoint{
				Method:        domain.NormalizeMethod(m.name),
				Path:          rawPath,
				Sources:       []string{"openapi"},
				PrimarySource: "openapi",
				Tags:          op.Tags,
				Spec:          specFromOperation(op),
			})
		}
	}
	return out
}

func specFromOperation(op *openapi3.Operation) *domain.EndpointSpec {
	spec := &domain.EndpointSpec{Name: op.OperationID}
	for _, pr := range op.Parameters {
		if pr.Value == nil {
			continue
		}
		typeName := ""
		if pr.Value.Schema != nil && pr.Value.Schema.Value != nil && len(pr.Value.Schema.Value.Type.Slice()) > 0 {
			typeName = pr.Value.Schema.Value.Type.Slice()[0]
		}
		spec.Parameters = append(spec.Parameters, domain.Parameter{
			Name:     pr.Value.Name,
			In:       pr.Value.In,
			Required: pr.Value.Required,
			Type:     typeName,
		})
	}
	if op.RequestBody != nil {
		spec.RequestBody = summarizeRequestBody(op.RequestBody)
	}
	if op.Responses != nil {
		spec.Responses = summarizeResponses(op.Responses)
	}
	return spec
}

// summarizeRequestBody/summarizeResponses keep only what generate/inspect
// need to display (content types + required flag / status codes), rather
// than holding onto the entire kin-openapi object graph
// (docs/07-discovery.md section 3: "Do not import vendor extensions unless
// needed").
func summarizeRequestBody(ref *openapi3.RequestBodyRef) map[string]any {
	if ref == nil || ref.Value == nil {
		return nil
	}
	contentTypes := make([]string, 0, len(ref.Value.Content))
	for ct := range ref.Value.Content {
		contentTypes = append(contentTypes, ct)
	}
	sort.Strings(contentTypes)
	return map[string]any{
		"required":      ref.Value.Required,
		"content_types": contentTypes,
	}
}

func summarizeResponses(responses *openapi3.Responses) map[string]any {
	out := map[string]any{}
	for code, ref := range responses.Map() {
		if ref == nil || ref.Value == nil {
			continue
		}
		desc := ""
		if ref.Value.Description != nil {
			desc = *ref.Value.Description
		}
		out[code] = desc
	}
	return out
}
