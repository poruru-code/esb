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
