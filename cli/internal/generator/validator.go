// Where: cli/internal/generator/schema/validator.go
// What: Schema validator for SAM templates.
// Why: Leverage the official AWS SAM JSON schema as a source of truth.
package generator

import (
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v5"
	"sigs.k8s.io/yaml"
)

var (
	schemaOnce     sync.Once
	schemaErr      error
	compiledSchema *jsonschema.Schema
)

func validateSAMTemplate(content []byte) ([]byte, error) {
	sch, err := loadSchema()
	if err != nil {
		return nil, err
	}

	jsonData, err := yaml.YAMLToJSON(content)
	if err != nil {
		return nil, fmt.Errorf("convert yaml to json: %w", err)
	}

	var document any
	if err := json.Unmarshal(jsonData, &document); err != nil {
		return nil, fmt.Errorf("marshal json: %w", err)
	}

	canonicalizeEnv(document)

	if err := sch.Validate(document); err != nil {
		return nil, err
	}
	return jsonData, nil
}

func loadSchema() (*jsonschema.Schema, error) {
	schemaOnce.Do(func() {
		path, err := schemaFilePath()
		if err != nil {
			schemaErr = err
			return
		}
		urlStr, err := fileURL(path)
		if err != nil {
			schemaErr = err
			return
		}
		compiler := jsonschema.NewCompiler()
		compiledSchema, schemaErr = compiler.Compile(urlStr)
	})
	return compiledSchema, schemaErr
}

func schemaFilePath() (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("unable to resolve schema path")
	}
	return filepath.Join(filepath.Dir(file), "schema", "sam.schema.json"), nil
}

func fileURL(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(abs)}).String(), nil
}

func canonicalizeEnv(document any) {
	root, ok := document.(map[string]any)
	if !ok {
		return
	}
	globalsObj, ok := root["Globals"].(map[string]any)
	if !ok {
		return
	}
	functionObj, ok := globalsObj["Function"].(map[string]any)
	if !ok {
		return
	}
	envObj, ok := functionObj["Environment"].(map[string]any)
	if !ok {
		return
	}
	variables, ok := envObj["Variables"].(map[string]any)
	if !ok {
		return
	}
	canonical := map[string]any{}
	for key, value := range variables {
		canonical[key] = fmt.Sprint(value)
	}
	envObj["Variables"] = canonical
}
