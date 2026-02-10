// Where: cli/internal/infra/build/image_import_types.go
// What: Image import manifest types used by config merge logic.
// Why: Keep merge schema stable while templategen owns image import generation.
package build

type imageImportManifest struct {
	Version    string             `json:"version"`
	PushTarget string             `json:"push_target"`
	Images     []imageImportEntry `json:"images"`
}

type imageImportEntry struct {
	FunctionName string `json:"function_name"`
	ImageSource  string `json:"image_source"`
	ImageRef     string `json:"image_ref"`
}
