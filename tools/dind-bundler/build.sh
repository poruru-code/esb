#!/usr/bin/env bash
# Where: tools/dind-bundler/build.sh
# What: Build a self-contained DinD bundle image.
# Why: Package prebuilt ESB images for offline/demo use.

set -euo pipefail

usage() {
  cat <<'USAGE'
Usage:
  ./tools/dind-bundler/build.sh <sam-template-path> [output-image-tag]
  ./tools/dind-bundler/build.sh -t <sam-template-path> [-t <sam-template-path> ...] [output-image-tag]

Examples:
  ./tools/dind-bundler/build.sh e2e/fixtures/template.core.yaml my-esb-bundle:latest
  ./tools/dind-bundler/build.sh -t template-a.yaml -t template-b.yaml my-esb-bundle:latest
USAGE
}

DEFAULTS_FILE="${DEFAULTS_FILE:-config/defaults.env}"
CLI_CMD=""
ENV_PREFIX=""
if [ -f "$DEFAULTS_FILE" ]; then
  CLI_CMD="$(awk -F= '/^CLI_CMD=/{print $2; exit}' "$DEFAULTS_FILE")"
  ENV_PREFIX="$(awk -F= '/^ENV_PREFIX=/{print $2; exit}' "$DEFAULTS_FILE")"
fi
CLI_CMD="${CLI_CMD:-esb}"
ENV_PREFIX="${ENV_PREFIX:-ESB}"
BRAND_SLUG="$(echo "$CLI_CMD" | tr '[:upper:]' '[:lower:]')"

TEMPLATES=()
OUTPUT_TAG=""
POSITIONAL=()

while [ "$#" -gt 0 ]; do
  case "$1" in
    -t|--template)
      if [ -z "${2:-}" ]; then
        echo "Error: --template requires a value."
        usage
        exit 1
      fi
      TEMPLATES+=("$2")
      shift 2
      ;;
    --template=*)
      TEMPLATES+=("${1#*=}")
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    --)
      shift
      POSITIONAL+=("$@")
      break
      ;;
    *)
      POSITIONAL+=("$1")
      shift
      ;;
  esac
done

if [ "${#TEMPLATES[@]}" -eq 0 ]; then
  if [ "${#POSITIONAL[@]}" -lt 1 ]; then
    echo "Error: TEMPLATE_PATH is required."
    usage
    exit 1
  fi
  TEMPLATES+=("${POSITIONAL[0]}")
  if [ "${#POSITIONAL[@]}" -ge 2 ]; then
    OUTPUT_TAG="${POSITIONAL[1]}"
  fi
  if [ "${#POSITIONAL[@]}" -gt 2 ]; then
    echo "Error: too many positional arguments."
    usage
    exit 1
  fi
else
  if [ "${#POSITIONAL[@]}" -eq 1 ]; then
    OUTPUT_TAG="${POSITIONAL[0]}"
  elif [ "${#POSITIONAL[@]}" -gt 1 ]; then
    echo "Error: too many positional arguments."
    usage
    exit 1
  fi
fi

if [ "${#TEMPLATES[@]}" -eq 0 ]; then
  echo "Error: TEMPLATE_PATH is required."
  usage
  exit 1
fi

OUTPUT_TAG=${OUTPUT_TAG:-"${BRAND_SLUG}-dind-bundle:latest"}
BUILD_DIR="tools/dind-bundler/build-context"
ENV_VAR="${ENV_PREFIX}_ENV"
ENV_NAME="${!ENV_VAR:-${ESB_ENV:-}}"
OUTPUT_VAR="${ENV_PREFIX}_OUTPUT_DIR"
OUTPUT_ROOTS=()
MANIFEST_PATHS=()
EXPLICIT_OUTPUT_ROOT="${!OUTPUT_VAR:-${ESB_OUTPUT_DIR:-}}"
declare -A OUTPUT_SUFFIX_COUNTS

expand_home() {
  case "$1" in
    "~")
      echo "$HOME"
      ;;
    "~"/*)
      echo "$HOME/${1#~/}"
      ;;
    *)
      echo "$1"
      ;;
  esac
}

if [ -n "${BUNDLE_MANIFEST_PATH:-}" ] && [ "${#TEMPLATES[@]}" -gt 1 ]; then
  echo "Error: BUNDLE_MANIFEST_PATH cannot be used with multiple templates."
  exit 1
fi

if [ -z "$ENV_NAME" ] && [ -f ".env" ]; then
  ENV_NAME="$(awk -F= '/^ENV=/{print $2; exit}' .env)"
fi
ENV_NAME="${ENV_NAME:-default}"

if [ -n "$EXPLICIT_OUTPUT_ROOT" ] && [ "${#TEMPLATES[@]}" -gt 1 ]; then
  echo "Error: ESB_OUTPUT_DIR cannot be used with multiple templates."
  exit 1
fi

for template in "${TEMPLATES[@]}"; do
  expanded="$(expand_home "$template")"
  if [ "${expanded#/}" != "$expanded" ]; then
    template_abs="$expanded"
  else
    template_abs="$(pwd)/$expanded"
  fi
  template_dir="$(cd "$(dirname "$template_abs")" && pwd)"
  default_output_root="${template_dir}/.${BRAND_SLUG}"
  output_root="$default_output_root"
  if [ -n "$EXPLICIT_OUTPUT_ROOT" ]; then
    output_root="$EXPLICIT_OUTPUT_ROOT"
  elif [ "${#TEMPLATES[@]}" -gt 1 ]; then
    template_base="$(basename "$template_abs")"
    template_stem="${template_base%.*}"
    if [ -z "$template_stem" ]; then
      template_stem="template"
    fi
    suffix_count="${OUTPUT_SUFFIX_COUNTS[$template_stem]:-0}"
    OUTPUT_SUFFIX_COUNTS[$template_stem]=$((suffix_count + 1))
    if [ "$suffix_count" -gt 0 ]; then
      template_stem="${template_stem}-$((suffix_count + 1))"
    fi
    output_root="${default_output_root}/${template_stem}"
  fi
  OUTPUT_ROOTS+=("$output_root")
  if [ -n "${BUNDLE_MANIFEST_PATH:-}" ]; then
    MANIFEST_PATHS+=("$BUNDLE_MANIFEST_PATH")
  else
    MANIFEST_PATHS+=("${output_root}/${ENV_NAME}/bundle/manifest.json")
  fi
done

CERT_DIR="${CERT_DIR:-$(pwd)/.${BRAND_SLUG}/certs}"
RUN_UID="${RUN_UID:-}"
RUN_GID="${RUN_GID:-}"
if [ -z "$RUN_UID" ] || [ -z "$RUN_GID" ]; then
  if [ -f ".env" ]; then
    if [ -z "$RUN_UID" ]; then
      RUN_UID="$(awk -F= '/^RUN_UID=/{print $2; exit}' .env)"
    fi
    if [ -z "$RUN_GID" ]; then
      RUN_GID="$(awk -F= '/^RUN_GID=/{print $2; exit}' .env)"
    fi
  fi
fi
RUN_UID="${RUN_UID:-1000}"
RUN_GID="${RUN_GID:-1000}"

# Ensure we are in the project root
if [ ! -f "cli/go.mod" ]; then
  echo "Error: Please run this script from the project root."
  exit 1
fi

echo "Building DinD bundle from templates: ${TEMPLATES[*]}"
echo "Output tag: $OUTPUT_TAG"
echo "Env: $ENV_NAME"
echo "Brand: $CLI_CMD"

if [ "${DIND_BUNDLER_DRYRUN:-}" = "true" ]; then
  join_by() { local IFS="$1"; shift; echo "$*"; }
  echo "CLI_CMD=$CLI_CMD"
  echo "ENV_PREFIX=$ENV_PREFIX"
  echo "BRAND_SLUG=$BRAND_SLUG"
  echo "TEMPLATES=$(join_by "," "${TEMPLATES[@]}")"
  echo "OUTPUT_TAG=$OUTPUT_TAG"
  echo "ENV_NAME=$ENV_NAME"
  echo "OUTPUT_ROOTS=$(join_by "," "${OUTPUT_ROOTS[@]}")"
  echo "MANIFEST_PATHS=$(join_by "," "${MANIFEST_PATHS[@]}")"
  echo "CERT_DIR=$CERT_DIR"
  echo "RUN_UID=$RUN_UID"
  echo "RUN_GID=$RUN_GID"
  exit 0
fi

# Check Certs (no fallback)
if [ ! -f "$CERT_DIR/rootCA.crt" ]; then
  echo "Error: Root CA not found at $CERT_DIR/rootCA.crt"
  echo "Hint: run 'mise run setup:certs' or set CERT_DIR."
  exit 1
fi

# Cleanup and setup build context
rm -rf "$BUILD_DIR"
mkdir -p "$BUILD_DIR"

# 1. Build Base Services and Functions using esb deploy (build-only)
for index in "${!TEMPLATES[@]}"; do
  template="${TEMPLATES[$index]}"
  output_root="${OUTPUT_ROOTS[$index]}"
  manifest_path="${MANIFEST_PATHS[$index]}"
  echo "Running esb deploy (build-only) for template: $template"
  deploy_output_args=()
  if [ -n "$EXPLICIT_OUTPUT_ROOT" ] || [ "${#TEMPLATES[@]}" -gt 1 ]; then
    deploy_output_args=(--output "$output_root")
  fi
  uv run "$CLI_CMD" deploy --template "$template" --env "$ENV_NAME" --mode docker --verbose --no-save-defaults --build-only --bundle-manifest "${deploy_output_args[@]}"
  # Verify manifest exists
  if [ ! -f "$manifest_path" ]; then
    echo "Error: Bundle manifest not found at $manifest_path"
    echo "Hint: run deploy with --bundle-manifest."
    exit 1
  fi
done

# 2. Merge manifests
mkdir -p "$BUILD_DIR/bundle"
MERGED_MANIFEST="$BUILD_DIR/bundle/manifest.json"
python3 tools/dind-bundler/merge_manifest.py --output "$MERGED_MANIFEST" "${MANIFEST_PATHS[@]}"

# 3. Load manifest (source of truth)
if [ ! -f "$MERGED_MANIFEST" ]; then
  echo "Error: Bundle manifest not found at $MERGED_MANIFEST"
  exit 1
fi

echo "Loading bundle manifest: $MERGED_MANIFEST"
IMAGES=$(python - <<'PY' "$MERGED_MANIFEST"
import json
import sys

path = sys.argv[1]
with open(path, "r", encoding="utf-8") as f:
    data = json.load(f)

images = data.get("images", [])
if not images:
    print("", end="")
    sys.exit(0)

for item in images:
    name = item.get("name")
    if name:
        print(name)
PY
)

if [ -z "$IMAGES" ]; then
  echo "Error: Manifest contains no images."
  exit 1
fi

# 4. Save Images
echo "Saving images to tarball..."
docker save -o "$BUILD_DIR/images.tar" $IMAGES

# 5. Prepare Context
echo "Copying config files..."
cp docker-compose.docker.yml "$BUILD_DIR/"
cp -r config "$BUILD_DIR/"
cp tools/dind-bundler/Dockerfile "$BUILD_DIR/"
cp tools/dind-bundler/entrypoint.sh "$BUILD_DIR/"

if [ -f ".env" ]; then
  cp ".env" "$BUILD_DIR/.env"
  if ! grep -q '^CERT_DIR=' "$BUILD_DIR/.env"; then
    echo "CERT_DIR=/root/.${BRAND_SLUG}/certs" >> "$BUILD_DIR/.env"
  fi
else
  {
    echo "ENV=$ENV_NAME"
    echo "RUSTFS_ACCESS_KEY=${RUSTFS_ACCESS_KEY:-esb}"
    echo "RUSTFS_SECRET_KEY=${RUSTFS_SECRET_KEY:-esbsecret}"
    echo "CERT_DIR=/root/.${BRAND_SLUG}/certs"
  } > "$BUILD_DIR/.env"
fi

# Copy merged manifest
cp "$MERGED_MANIFEST" "$BUILD_DIR/bundle/manifest.json"

# Copy certs
echo "Copying certificates..."
mkdir -p "$BUILD_DIR/certs"
cp "$CERT_DIR"/* "$BUILD_DIR/certs/"

# 6. Build DinD Image
echo "Building DinD image..."
docker build -t "$OUTPUT_TAG" \
  --build-arg BRAND_HOME=".${BRAND_SLUG}" \
  --build-arg CERT_UID="$RUN_UID" \
  --build-arg CERT_GID="$RUN_GID" \
  "$BUILD_DIR"

echo "Done! Image $OUTPUT_TAG created."
