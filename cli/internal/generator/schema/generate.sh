#!/bin/bash
set -e

# This script regenerates the SAM Go models from the JSON schema.
# It uses elastic/go-json-schema-generate (managed by mise) to produce lightweight structs without strict validation code.

# Ensure we are in the script directory
cd "$(dirname "$0")"

SCHEMA_FILE="sam.schema.json"
OUTPUT_FILE="sam_generated.go"

# Check if schema-generate is available (should be provided by mise)
if ! command -v schema-generate &> /dev/null; then
    echo "Error: schema-generate not found. Please run 'mise install' to install dependencies."
    exit 1
fi

echo "Generating Go models from $SCHEMA_FILE..."
# -p schema: Package name
# -i $SCHEMA_FILE: Input schema
# -o $OUTPUT_FILE: Output file
# -s: Skip marshaling/unmarshaling code generation (crucial for flexibility and size reduction)
schema-generate -p schema -i "$SCHEMA_FILE" -o "$OUTPUT_FILE" -s

echo "Post-processing: Removing unused imports..."
# The -s flag leaves an unused "encoding/json" import. We remove it to fix build errors.
# Using a temporary file for cross-platform compatibility (sed -i behaves differently on Mac/Linux)
grep -v '"encoding/json"' "$OUTPUT_FILE" > "${OUTPUT_FILE}.tmp" && mv "${OUTPUT_FILE}.tmp" "$OUTPUT_FILE"

echo "Successfully regenerated $OUTPUT_FILE"
