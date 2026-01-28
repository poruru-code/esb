// Where: tools/traceability/docker-bake.hcl
// What: Bake target to generate version.json once per build.
// Why: Provide a shared metadata context for all image builds.

target "meta" {
  context    = "."
  dockerfile = "tools/traceability/Dockerfile.meta"
  target     = "out"
  contexts = {
    git_dir    = ".git"
    git_common = ".git"
    trace_tools = "tools/traceability"
  }
  output = ["type=local,dest=.esb/meta"]
}

target "meta-image" {
  context    = "."
  dockerfile = "tools/traceability/Dockerfile.meta"
  target     = "meta"
  contexts = {
    git_dir    = ".git"
    git_common = ".git"
    trace_tools = "tools/traceability"
  }
  tags     = ["esb-meta:debug"]
  output   = ["type=docker"]
}

// Base/control images (targets are configured at runtime via bake overrides).
target "lambda-base" {
  context    = "cli/internal/generator/assets"
  dockerfile = "Dockerfile.lambda-base"
  contexts = {
    meta = "target:meta"
  }
}

target "os-base" {
  context    = "services/common"
  dockerfile = "Dockerfile.os-base"
  contexts = {
    meta = "target:meta"
  }
  tags = ["esb-os-base:latest"]
  output = ["type=docker"]
  cache-from = ["type=local,src=.esb/buildx-cache/base"]
  cache-to   = ["type=local,dest=.esb/buildx-cache/base,mode=max"]
}

target "python-base" {
  context    = "services/common"
  dockerfile = "Dockerfile.python-base"
  contexts = {
    meta = "target:meta"
  }
  tags = ["esb-python-base:latest"]
  output = ["type=docker"]
  cache-from = ["type=local,src=.esb/buildx-cache/base"]
  cache-to   = ["type=local,dest=.esb/buildx-cache/base,mode=max"]
}

group "base-images" {
  targets = ["lambda-base", "os-base", "python-base"]
}
