package engine

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type imageImportManifest struct {
	Version    string             `json:"version"`
	PushTarget string             `json:"push_target,omitempty"`
	Images     []imageImportEntry `json:"images"`
}

type imageImportEntry struct {
	FunctionName string `json:"function_name,omitempty"`
	ImageSource  string `json:"image_source,omitempty"`
	ImageRef     string `json:"image_ref"`
}

func loadYAML(path string) (map[string]any, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, false, nil
		}
		return nil, false, err
	}
	result := map[string]any{}
	if err := yaml.Unmarshal(data, &result); err != nil {
		return nil, false, err
	}
	return result, true, nil
}

func loadImageImportManifest(path string) (imageImportManifest, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return imageImportManifest{}, false, nil
		}
		return imageImportManifest{}, false, err
	}
	var result imageImportManifest
	if err := json.Unmarshal(data, &result); err != nil {
		return imageImportManifest{}, false, err
	}
	if result.Images == nil {
		result.Images = []imageImportEntry{}
	}
	return result, true, nil
}

func atomicWriteYAML(path string, data map[string]any) error {
	content, err := marshalYAML(data, 2)
	if err != nil {
		return err
	}
	return atomicWriteFile(path, content)
}

func marshalYAML(value any, indent int) ([]byte, error) {
	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(indent)
	if err := encoder.Encode(value); err != nil {
		_ = encoder.Close()
		return nil, err
	}
	if err := encoder.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func atomicWriteJSON(path string, value any) error {
	content, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	content = append(content, '\n')
	return atomicWriteFile(path, content)
}

func atomicWriteFile(path string, content []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	cleanup := func() {
		_ = os.Remove(tmpPath)
	}
	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		cleanup()
		return err
	}
	return nil
}
