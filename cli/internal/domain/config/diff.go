// Where: cli/internal/domain/config/diff.go
// What: Pure config diff helpers for deploy summaries.
// Why: Keep config diff logic independent from I/O.
package config

import (
	"fmt"
	"reflect"
)

// Snapshot captures the normalized config state used for diffing.
type Snapshot struct {
	Functions map[string]any
	Routes    map[string]any
	Resources map[string]map[string]any
}

// Counts stores diff counters for a category.
type Counts struct {
	Added   int
	Updated int
	Removed int
	Total   int
}

// Diff aggregates counts for all config sections.
type Diff struct {
	Functions Counts
	Routes    Counts
	Resources map[string]Counts
}

// DiffSnapshots computes the diff between two snapshots.
func DiffSnapshots(before, after Snapshot) Diff {
	diff := Diff{
		Functions: diffMap(before.Functions, after.Functions),
		Routes:    diffMap(before.Routes, after.Routes),
		Resources: map[string]Counts{},
	}
	for _, key := range []string{"dynamodb", "s3", "layers"} {
		diff.Resources[key] = diffMap(resourceMap(before, key), resourceMap(after, key))
	}
	return diff
}

// FormatCountsLabel formats counts for merged config summaries.
func FormatCountsLabel(counts Counts) string {
	return fmt.Sprintf(
		"new %d / updated %d / removed %d (total %d)",
		counts.Added,
		counts.Updated,
		counts.Removed,
		counts.Total,
	)
}

// FormatTemplateCounts formats counts for template delta summaries.
func FormatTemplateCounts(counts Counts) string {
	unchanged := counts.Total - counts.Added - counts.Updated
	if unchanged < 0 {
		unchanged = 0
	}
	return fmt.Sprintf(
		"new %d / updated %d / unchanged %d (template %d)",
		counts.Added,
		counts.Updated,
		unchanged,
		counts.Total,
	)
}

func diffMap(before, after map[string]any) Counts {
	counts := Counts{Total: len(after)}
	if before == nil {
		before = map[string]any{}
	}
	if after == nil {
		after = map[string]any{}
	}
	for key, value := range after {
		prev, ok := before[key]
		if !ok {
			counts.Added++
			continue
		}
		if !reflect.DeepEqual(prev, value) {
			counts.Updated++
		}
	}
	for key := range before {
		if _, ok := after[key]; !ok {
			counts.Removed++
		}
	}
	return counts
}

func resourceMap(snapshot Snapshot, key string) map[string]any {
	if snapshot.Resources == nil {
		return map[string]any{}
	}
	if resources, ok := snapshot.Resources[key]; ok {
		return resources
	}
	return map[string]any{}
}
