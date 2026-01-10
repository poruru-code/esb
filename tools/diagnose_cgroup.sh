#!/usr/bin/env bash
# Where: tools/diagnose_cgroup.sh
# What: Collect host/runtime-node cgroup and containerd diagnostics in one run.
# Why: Provide a single report to pinpoint cgroup v2/namespace issues.
set -euo pipefail

print_section() {
  printf "\n==== %s ====\n" "$1"
}

safe_cat() {
  local path="$1"
  if [ -f "$path" ]; then
    printf "%s: " "$path"
    cat "$path"
  else
    printf "%s: (missing)\n" "$path"
  fi
}

print_section "Host: OS and Kernel"
uname -a || true
safe_cat /proc/cmdline
if [ -f /etc/os-release ]; then
  printf "os-release:\n"
  cat /etc/os-release
fi
if [ -f /etc/wsl.conf ]; then
  printf "wsl.conf:\n"
  cat /etc/wsl.conf
fi
printf "init: "
ps -p 1 -o comm= 2>/dev/null || true

print_section "Host: cgroup v2"
if [ -f /sys/fs/cgroup/cgroup.controllers ]; then
  safe_cat /sys/fs/cgroup/cgroup.controllers
  safe_cat /sys/fs/cgroup/cgroup.subtree_control
  safe_cat /sys/fs/cgroup/cgroup.type
  printf "cgroup2 mount:\n"
  grep -E "cgroup2" /proc/self/mountinfo || true
  printf "top-level entries (first 50):\n"
  ls -1 /sys/fs/cgroup | head -n 50
else
  printf "/sys/fs/cgroup/cgroup.controllers not found (cgroup v2 not detected)\n"
fi

print_section "Host: docker"
if command -v docker >/dev/null 2>&1; then
  docker info --format 'CgroupDriver={{.CgroupDriver}} CgroupVersion={{.CgroupVersion}}' || true
  docker version --format 'Server={{.Server.Version}} Client={{.Client.Version}}' || true
  printf "runtime-node containers:\n"
  docker ps --filter name=runtime-node --format '{{.Names}}' || true
else
  printf "docker not found\n"
fi

runtime_nodes=()
if command -v docker >/dev/null 2>&1; then
  while IFS= read -r name; do
    [ -n "$name" ] && runtime_nodes+=("$name")
  done < <(docker ps --filter name=runtime-node --format '{{.Names}}')
fi

for node in "${runtime_nodes[@]}"; do
  print_section "runtime-node: $node (container inspect)"
  docker inspect "$node" --format 'Name={{.Name}} Privileged={{.HostConfig.Privileged}} CgroupnsMode={{.HostConfig.CgroupnsMode}} CgroupParent={{.HostConfig.CgroupParent}}' || true

  print_section "runtime-node: $node (inside container)"
  docker exec "$node" sh -lc '
set -e
echo "proc_self_cgroup: $(cat /proc/self/cgroup)"
echo "proc_1_cgroup: $(cat /proc/1/cgroup)"
echo "cgroup.controllers: $(cat /sys/fs/cgroup/cgroup.controllers 2>/dev/null || echo missing)"
echo "cgroup.subtree_control: $(cat /sys/fs/cgroup/cgroup.subtree_control 2>/dev/null || echo missing)"
echo "cgroup.type: $(cat /sys/fs/cgroup/cgroup.type 2>/dev/null || echo missing)"
if [ -f /sys/fs/cgroup/cgroup.procs ]; then
  echo "cgroup.procs count: $(wc -l /sys/fs/cgroup/cgroup.procs | awk '\''{print $1}'\'')"
fi
if [ -f /sys/fs/cgroup/cgroup.threads ]; then
  echo "cgroup.threads count: $(wc -l /sys/fs/cgroup/cgroup.threads | awk '\''{print $1}'\'')"
fi
if [ -d /sys/fs/cgroup/esb ]; then
  echo "esb/cgroup.type: $(cat /sys/fs/cgroup/esb/cgroup.type 2>/dev/null || echo missing)"
  echo "esb/cgroup.subtree_control: $(cat /sys/fs/cgroup/esb/cgroup.subtree_control 2>/dev/null || echo missing)"
  echo "esb controllers present: $(ls /sys/fs/cgroup/esb 2>/dev/null | grep -E "^memory" | paste -sd " " - || echo none)"
  if [ -f /sys/fs/cgroup/esb/cgroup.procs ]; then
    echo "esb/cgroup.procs count: $(wc -l /sys/fs/cgroup/esb/cgroup.procs | awk '\''{print $1}'\'')"
  fi
  if [ -f /sys/fs/cgroup/esb/cgroup.threads ]; then
    echo "esb/cgroup.threads count: $(wc -l /sys/fs/cgroup/esb/cgroup.threads | awk '\''{print $1}'\'')"
  fi
  echo "esb subtree types:"
  find /sys/fs/cgroup/esb -maxdepth 2 -name cgroup.type -print -exec cat {} \; 2>/dev/null || true
fi
echo "esb-prefixed dirs:"
ls -1 /sys/fs/cgroup | grep -E "^esb" || true
'

  print_section "runtime-node: $node (containerd config)"
  docker exec "$node" sh -lc '
if command -v containerd >/dev/null 2>&1; then
  containerd config dump 2>/dev/null | grep -n "SystemdCgroup" || true
  containerd config dump 2>/dev/null | grep -n "cgroup" | head -n 20 || true
else
  echo "containerd not found"
fi
if command -v runc >/dev/null 2>&1; then
  runc --version | head -n 1 || true
fi
'
done

print_section "Host: recent kernel/cgroup hints"
if command -v dmesg >/dev/null 2>&1; then
  dmesg | tail -n 50 || true
else
  printf "dmesg not available\n"
fi
