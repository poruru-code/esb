package config

import "testing"

func TestDiffSnapshots(t *testing.T) {
	before := Snapshot{
		Functions: map[string]any{
			"unchanged": map[string]any{"handler": "a.handler"},
			"updated":   map[string]any{"handler": "old.handler"},
			"removed":   map[string]any{"handler": "remove.handler"},
		},
		Routes: map[string]any{
			"GET /health": "unchanged",
		},
		Resources: map[string]map[string]any{
			"dynamodb": {
				"table_old": map[string]any{"name": "table_old"},
			},
		},
	}
	after := Snapshot{
		Functions: map[string]any{
			"unchanged": map[string]any{"handler": "a.handler"},
			"updated":   map[string]any{"handler": "new.handler"},
			"added":     map[string]any{"handler": "added.handler"},
		},
		Routes: map[string]any{
			"GET /health": "unchanged",
			"POST /items": "added",
		},
		Resources: map[string]map[string]any{
			"dynamodb": {
				"table_old": map[string]any{"name": "table_old_v2"},
			},
			"s3": {
				"bucket_new": map[string]any{"name": "bucket_new"},
			},
		},
	}

	diff := DiffSnapshots(before, after)

	if diff.Functions.Added != 1 || diff.Functions.Updated != 1 || diff.Functions.Removed != 1 || diff.Functions.Total != 3 {
		t.Fatalf("functions diff mismatch: %#v", diff.Functions)
	}
	if diff.Routes.Added != 1 || diff.Routes.Updated != 0 || diff.Routes.Removed != 0 || diff.Routes.Total != 2 {
		t.Fatalf("routes diff mismatch: %#v", diff.Routes)
	}
	if diff.Resources["dynamodb"].Added != 0 || diff.Resources["dynamodb"].Updated != 1 || diff.Resources["dynamodb"].Removed != 0 || diff.Resources["dynamodb"].Total != 1 {
		t.Fatalf("dynamodb diff mismatch: %#v", diff.Resources["dynamodb"])
	}
	if diff.Resources["s3"].Added != 1 || diff.Resources["s3"].Updated != 0 || diff.Resources["s3"].Removed != 0 || diff.Resources["s3"].Total != 1 {
		t.Fatalf("s3 diff mismatch: %#v", diff.Resources["s3"])
	}
	if diff.Resources["layers"] != (Counts{}) {
		t.Fatalf("layers diff mismatch: %#v", diff.Resources["layers"])
	}
}

func TestFormatCountsLabel(t *testing.T) {
	got := FormatCountsLabel(Counts{Added: 2, Updated: 1, Removed: 3, Total: 8})
	want := "new 2 / updated 1 / removed 3 (total 8)"
	if got != want {
		t.Fatalf("FormatCountsLabel() = %q, want %q", got, want)
	}
}

func TestFormatTemplateCounts(t *testing.T) {
	t.Run("normal", func(t *testing.T) {
		got := FormatTemplateCounts(Counts{Added: 1, Updated: 2, Total: 10})
		want := "new 1 / updated 2 / unchanged 7 (template 10)"
		if got != want {
			t.Fatalf("FormatTemplateCounts() = %q, want %q", got, want)
		}
	})

	t.Run("unchanged floor is zero", func(t *testing.T) {
		got := FormatTemplateCounts(Counts{Added: 5, Updated: 7, Total: 2})
		want := "new 5 / updated 7 / unchanged 0 (template 2)"
		if got != want {
			t.Fatalf("FormatTemplateCounts() = %q, want %q", got, want)
		}
	})
}

func TestDiffSnapshotsHandlesNilMaps(t *testing.T) {
	diff := DiffSnapshots(Snapshot{}, Snapshot{})
	if diff.Functions != (Counts{}) {
		t.Fatalf("functions diff = %#v, want zero", diff.Functions)
	}
	if diff.Routes != (Counts{}) {
		t.Fatalf("routes diff = %#v, want zero", diff.Routes)
	}
	for _, key := range []string{"dynamodb", "s3", "layers"} {
		if diff.Resources[key] != (Counts{}) {
			t.Fatalf("resource %s diff = %#v, want zero", key, diff.Resources[key])
		}
	}
}
