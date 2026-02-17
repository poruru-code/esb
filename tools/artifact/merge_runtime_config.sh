#!/usr/bin/env bash
set -euo pipefail

ARTIFACT=""
OUT=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --artifact)
      ARTIFACT="$2"
      shift 2
      ;;
    --out)
      OUT="$2"
      shift 2
      ;;
    *)
      echo "unknown arg: $1" >&2
      exit 1
      ;;
  esac
done

if [[ -z "${ARTIFACT}" ]]; then
  echo "--artifact is required" >&2
  exit 1
fi
if [[ -z "${OUT}" ]]; then
  echo "--out is required" >&2
  exit 1
fi

exec artifactctl merge --artifact "${ARTIFACT}" --out "${OUT}"
