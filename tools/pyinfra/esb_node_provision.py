from pyinfra import host
from pyinfra.operations import apt, files, server

DEFAULT_PACKAGES = [
    "ca-certificates",
    "containerd",
    "curl",
    "dmsetup",
    "docker.io",
    "gcc",
    "git",
    "golang-go",
    "iproute2",
    "iptables",
    "lvm2",
    "make",
    "qemu-utils",
    "socat",
    "sudo",
    "util-linux",
]

DEFAULT_GROUPS = ["kvm", "docker"]


def _get_list(data_key: str, fallback: list[str]) -> list[str]:
    value = host.data.get(data_key)
    if isinstance(value, list) and value:
        return value
    return fallback


def _get_bool(data_key: str, fallback: bool) -> bool:
    value = host.data.get(data_key)
    if value is None:
        return fallback
    if isinstance(value, bool):
        return value
    if isinstance(value, int):
        return value != 0
    if isinstance(value, str):
        return value.strip().lower() in {"1", "true", "yes", "on"}
    return fallback


def _write_file_command(path: str, content: str, mode: str = "0644") -> str:
    content = content.rstrip("\n")
    lines = [
        "set -eu",
        f"cat > \"{path}\" <<'EOF'",
        content,
        "EOF",
        f"chmod {mode} \"{path}\"",
    ]
    return "\n".join(lines)


FIRECRACKER_FIFO_PATCH = """diff --git a/runtime/service.go b/runtime/service.go
index 979e04b..1b04ae9 100644
--- a/runtime/service.go
+++ b/runtime/service.go
@@ -18,6 +18,7 @@ import (
 \t\"encoding/json\"
 \t\"errors\"
 \t\"fmt\"
+\t\"io\"
 \t\"math\"
 \t\"net\"
 \t\"os\"
@@ -25,7 +26,6 @@ import (
 \t\"strconv\"
 \t\"strings\"
 \t\"sync\"
-\t\"syscall\"
 \t\"time\"
 
 \t// disable gosec check for math/rand. We just need a random starting
@@ -964,24 +964,19 @@ func (s *service) buildVMConfiguration(req *proto.CreateVMRequest) (*firecracker
 \tif req.LogFifoPath != \"\" {
 \t\tlogPath = req.LogFifoPath
 \t}
-\terr = syscall.Mkfifo(logPath, 0700)
-\tif err != nil {
-\t\treturn nil, err
-\t}
-
-\tmetricsPath := s.shimDir.FirecrackerMetricsFifoPath()
-\tif req.MetricsFifoPath != \"\" {
-\t\tmetricsPath = req.MetricsFifoPath
-\t}
-\terr = syscall.Mkfifo(metricsPath, 0700)
-\tif err != nil {
-\t\treturn nil, err
-\t}
 
 \t// The Config struct has LogFifo and MetricsFifo, but they will be deprecated since
 \t// Firecracker doesn't have the corresponding fields anymore.
 \tcfg.LogPath = logPath
-\tcfg.MetricsPath = metricsPath
+\tcfg.LogFifo = logPath
+\t// Keep a reader attached so Firecracker can open the fifo without ENXIO.
+\tcfg.FifoLogWriter = io.Discard
+
+\tif req.MetricsFifoPath != \"\" {
+\t\tmetricsPath := req.MetricsFifoPath
+\t\tcfg.MetricsPath = metricsPath
+\t\tcfg.MetricsFifo = metricsPath
+\t}
 
 \tif req.JailerConfig != nil {
 \t\tcfg.NetNS = req.JailerConfig.NetNS
"""


packages = _get_list("esb_packages", DEFAULT_PACKAGES)
groups = _get_list("esb_groups", DEFAULT_GROUPS)
user = host.data.get("esb_user") or host.data.get("ssh_user") or "root"
sudo_nopasswd = bool(host.data.get("esb_sudo_nopasswd"))
firecracker_version = host.data.get("esb_firecracker_version") or "1.14.0"
firecracker_containerd_ref = (
    host.data.get("esb_firecracker_containerd_ref")
    or "d6ffdaa615b8da95dea35e611d0290447fcf8fd1"
)
firecracker_install_dir = host.data.get("esb_firecracker_install_dir") or "/usr/local/bin"
firecracker_runtime_dir = host.data.get("esb_firecracker_runtime_dir") or "/var/lib/firecracker-containerd/runtime"
firecracker_source_dir = host.data.get("esb_firecracker_source_dir") or (
    f"/home/{user}/firecracker-containerd" if user and user != "root" else "/root/firecracker-containerd"
)
firecracker_kernel_path = host.data.get("esb_firecracker_kernel_path") or f"{firecracker_runtime_dir}/default-vmlinux.bin"
firecracker_rootfs_path = host.data.get("esb_firecracker_rootfs_path") or f"{firecracker_runtime_dir}/default-rootfs.img"
firecracker_kernel_url = host.data.get("esb_firecracker_kernel_url") or ""
firecracker_rootfs_url = host.data.get("esb_firecracker_rootfs_url") or ""
firecracker_kernel_marker = f"{firecracker_runtime_dir}/.kernel.source"
firecracker_kernel_sha_marker = f"{firecracker_runtime_dir}/.kernel.sha256"
firecracker_rootfs_marker = f"{firecracker_runtime_dir}/.rootfs.source"
firecracker_rootfs_sha_marker = f"{firecracker_runtime_dir}/.rootfs.sha256"

devmapper_pool = host.data.get("esb_devmapper_pool") or "fc-dev-pool2"
devmapper_dir = host.data.get("esb_devmapper_dir") or "/var/lib/containerd/devmapper2"
devmapper_data_size = host.data.get("esb_devmapper_data_size") or "10G"
devmapper_meta_size = host.data.get("esb_devmapper_meta_size") or "2G"
devmapper_base_image_size = host.data.get("esb_devmapper_base_image_size") or "10GB"
devmapper_udev = _get_bool("esb_devmapper_udev", True)


apt.packages(
    name="Install base packages for ESB node",
    packages=packages,
    update=True,
)

for group in groups:
    server.group(
        name=f"Ensure group {group} exists",
        group=group,
    )

if user and user != "root":
    server.user(
        name=f"Ensure {user} is in required groups",
        user=user,
        groups=groups,
        append=True,
        ensure_home=False,
        create_home=False,
    )

if sudo_nopasswd and user and user != "root":
    server.shell(
        name=f"Allow passwordless sudo for {user}",
        commands=[
            "\n".join(
                [
                    "set -eu",
                    "install -d -m 0755 /etc/sudoers.d",
                    _write_file_command(
                        f"/etc/sudoers.d/esb-{user}",
                        f"{user} ALL=(ALL) NOPASSWD:ALL\n",
                        mode="0440",
                    ),
                ]
            )
        ],
    )

files.directory(
    name="Ensure Firecracker runtime directory exists",
    path=firecracker_runtime_dir,
    present=True,
    recursive=True,
)

files.directory(
    name="Ensure Firecracker install directory exists",
    path=firecracker_install_dir,
    present=True,
    recursive=True,
)

server.shell(
    name="Stage firecracker fifo patch",
    commands=[
        _write_file_command(
            "/tmp/firecracker-fifo-reader.patch",
            FIRECRACKER_FIFO_PATCH,
            mode="0644",
        )
    ],
)

server.shell(
    name="Install Firecracker binaries",
    commands=[
        f"""
set -eu
if [ -x "{firecracker_install_dir}/firecracker" ] && "{firecracker_install_dir}/firecracker" --version 2>/dev/null | grep -q "v{firecracker_version}"; then
  exit 0
fi

tmpdir="$(mktemp -d)"
arch="$(uname -m)"
case "$arch" in
  amd64|x86_64) arch="x86_64" ;;
  arm64|aarch64) arch="aarch64" ;;
  *) echo "unsupported architecture: $arch" >&2; exit 1 ;;
esac

url="https://github.com/firecracker-microvm/firecracker/releases/download/v{firecracker_version}/firecracker-v{firecracker_version}-${{arch}}.tgz"
curl -fsSL -o "$tmpdir/firecracker.tgz" "$url"
tar -xzf "$tmpdir/firecracker.tgz" -C "$tmpdir"
dir="$tmpdir/release-v{firecracker_version}-${{arch}}"
install -m 0755 "$dir/firecracker-v{firecracker_version}-${{arch}}" "{firecracker_install_dir}/firecracker"
install -m 0755 "$dir/jailer-v{firecracker_version}-${{arch}}" "{firecracker_install_dir}/jailer"
rm -rf "$tmpdir"
"""
    ],
)

server.shell(
    name="Install firecracker-containerd and shim",
    commands=[
        f"""
set -eu
marker="/usr/local/share/esb/firecracker-containerd-build"
build_id="{firecracker_containerd_ref}-fifo"
if [ -f "$marker" ] && grep -q "$build_id" "$marker"; then
  exit 0
fi

tmpdir="$(mktemp -d)"
git init "$tmpdir"
cd "$tmpdir"
git remote add origin https://github.com/firecracker-microvm/firecracker-containerd.git
git fetch --depth 1 origin "{firecracker_containerd_ref}"
git checkout FETCH_HEAD
patch="/tmp/firecracker-fifo-reader.patch"
if [ -f "$patch" ]; then
  if git apply --check "$patch" >/dev/null 2>&1; then
    git apply "$patch"
  fi
fi
make -C runtime containerd-shim-aws-firecracker
make -C firecracker-control/cmd/containerd firecracker-containerd firecracker-ctr
install -m 0755 runtime/containerd-shim-aws-firecracker "{firecracker_install_dir}/containerd-shim-aws-firecracker"
install -m 0755 firecracker-control/cmd/containerd/firecracker-containerd "{firecracker_install_dir}/firecracker-containerd"
install -m 0755 firecracker-control/cmd/containerd/firecracker-ctr "{firecracker_install_dir}/firecracker-ctr"
install -d -m 0755 /usr/local/share/esb
echo "$build_id" > "$marker"
rm -rf "$tmpdir"
"""
    ],
)

files.directory(
    name="Ensure firecracker-containerd config directory exists",
    path="/etc/firecracker-containerd",
    present=True,
    recursive=True,
)

files.directory(
    name="Ensure containerd config directory exists",
    path="/etc/containerd",
    present=True,
    recursive=True,
)

firecracker_containerd_config = (
    "version = 2\n"
    "disabled_plugins = [\"io.containerd.grpc.v1.cri\"]\n"
    "root = \"/var/lib/containerd\"\n"
    "state = \"/run/containerd\"\n"
    "\n"
    "[grpc]\n"
    "  address = \"/run/containerd/containerd.sock\"\n"
    "\n"
    "[plugins]\n"
    "  [plugins.\"io.containerd.snapshotter.v1.devmapper\"]\n"
    f"    pool_name = \"{devmapper_pool}\"\n"
    f"    base_image_size = \"{devmapper_base_image_size}\"\n"
    f"    root_path = \"{devmapper_dir}\"\n"
    "\n"
    "[debug]\n"
    "  level = \"debug\"\n"
)
server.shell(
    name="Write firecracker-containerd config",
    commands=[_write_file_command("/etc/firecracker-containerd/config.toml", firecracker_containerd_config)],
)

firecracker_runtime_config = (
    "{\n"
    f"  \"firecracker_binary_path\": \"{firecracker_install_dir}/firecracker\",\n"
    f"  \"kernel_image_path\": \"{firecracker_kernel_path}\",\n"
    "  \"kernel_args\": \"ro console=ttyS0 noapic reboot=k panic=1 pci=off nomodules "
    "systemd.unified_cgroup_hierarchy=0 systemd.journald.forward_to_console "
    "systemd.unit=firecracker.target init=/sbin/overlay-init\",\n"
    f"  \"root_drive\": \"{firecracker_rootfs_path}\",\n"
    "  \"cpu_template\": \"T2\",\n"
    "  \"log_levels\": [\"info\"],\n"
    "  \"jailer\": {\n"
    "    \"runc_binary_path\": \"/usr/bin/runc\"\n"
    "  }\n"
    "}\n"
)
server.shell(
    name="Write firecracker runtime config",
    commands=[_write_file_command("/etc/containerd/firecracker-runtime.json", firecracker_runtime_config)],
)

firecracker_runc_config = (
    "{\n"
    "  \"ociVersion\": \"1.0.1\",\n"
    "  \"process\": {\n"
    "    \"terminal\": false,\n"
    "    \"user\": {\n"
    "      \"uid\": 0,\n"
    "      \"gid\": 0\n"
    "    },\n"
    "    \"args\": [\n"
    "      \"/firecracker\",\n"
    "      \"--api-sock\",\n"
    "      \"api.socket\"\n"
    "    ],\n"
    "    \"env\": [\n"
    "      \"PATH=/\"\n"
    "    ],\n"
    "    \"cwd\": \"/\",\n"
    "    \"capabilities\": {\n"
    "      \"effective\": [],\n"
    "      \"bounding\": [],\n"
    "      \"inheritable\": [],\n"
    "      \"permitted\": [],\n"
    "      \"ambient\": []\n"
    "    },\n"
    "    \"rlimits\": [\n"
    "      {\n"
    "        \"type\": \"RLIMIT_NOFILE\",\n"
    "        \"hard\": 1024,\n"
    "        \"soft\": 1024\n"
    "      }\n"
    "    ],\n"
    "    \"noNewPrivileges\": true\n"
    "  },\n"
    "  \"root\": {\n"
    "    \"path\": \"rootfs\",\n"
    "    \"readonly\": false\n"
    "  },\n"
    "  \"hostname\": \"runc\",\n"
    "  \"mounts\": [\n"
    "    {\n"
    "      \"destination\": \"/proc\",\n"
    "      \"type\": \"proc\",\n"
    "      \"source\": \"proc\"\n"
    "    }\n"
    "  ],\n"
    "  \"linux\": {\n"
    "    \"devices\": [\n"
    "      {\n"
    "        \"path\": \"/dev/kvm\",\n"
    "        \"type\": \"c\",\n"
    "        \"major\": 10,\n"
    "        \"minor\": 232,\n"
    "        \"fileMode\": 438,\n"
    "        \"uid\": 0,\n"
    "        \"gid\": 0\n"
    "      },\n"
    "      {\n"
    "        \"path\": \"/dev/net/tun\",\n"
    "        \"type\": \"c\",\n"
    "        \"major\": 10,\n"
    "        \"minor\": 200,\n"
    "        \"fileMode\": 438,\n"
    "        \"uid\": 0,\n"
    "        \"gid\": 0\n"
    "      }\n"
    "    ],\n"
    "    \"resources\": {\n"
    "      \"devices\": [\n"
    "        {\n"
    "          \"allow\": false,\n"
    "          \"access\": \"rwm\"\n"
    "        },\n"
    "        {\n"
    "          \"allow\": true,\n"
    "          \"major\": 10,\n"
    "          \"minor\": 232,\n"
    "          \"access\": \"rwm\"\n"
    "        },\n"
    "        {\n"
    "          \"allow\": true,\n"
    "          \"major\": 10,\n"
    "          \"minor\": 200,\n"
    "          \"access\": \"rwm\"\n"
    "        }\n"
    "      ]\n"
    "    },\n"
    "    \"namespaces\": [\n"
    "      {\n"
    "        \"type\": \"cgroup\"\n"
    "      },\n"
    "      {\n"
    "        \"type\": \"pid\"\n"
    "      },\n"
    "      {\n"
    "        \"type\": \"network\"\n"
    "      },\n"
    "      {\n"
    "        \"type\": \"ipc\"\n"
    "      },\n"
    "      {\n"
    "        \"type\": \"uts\"\n"
    "      },\n"
    "      {\n"
    "        \"type\": \"mount\"\n"
    "      }\n"
    "    ],\n"
    "    \"maskedPaths\": [\n"
    "      \"/proc/asound\",\n"
    "      \"/proc/kcore\",\n"
    "      \"/proc/latency_stats\",\n"
    "      \"/proc/timer_list\",\n"
    "      \"/proc/timer_stats\",\n"
    "      \"/proc/sched_debug\",\n"
    "      \"/sys/firmware\",\n"
    "      \"/proc/scsi\"\n"
    "    ],\n"
    "    \"readonlyPaths\": [\n"
    "      \"/proc/bus\",\n"
    "      \"/proc/fs\",\n"
    "      \"/proc/irq\",\n"
    "      \"/proc/sys\",\n"
    "      \"/proc/sysrq-trigger\"\n"
    "    ]\n"
    "  }\n"
    "}\n"
)
server.shell(
    name="Write firecracker runc config",
    commands=[_write_file_command("/etc/containerd/firecracker-runc-config.json", firecracker_runc_config)],
)

server.shell(
    name="Download Firecracker kernel",
    commands=[
        f"""
set -eu
kernel_url="{firecracker_kernel_url}"
kernel_path="{firecracker_kernel_path}"
kernel_source_marker="{firecracker_kernel_marker}"
kernel_sha_marker="{firecracker_kernel_sha_marker}"

arch="$(uname -m)"
case "$arch" in
  amd64|x86_64) arch="x86_64" ;;
  arm64|aarch64) arch="aarch64" ;;
  *) echo "unsupported architecture: $arch" >&2; exit 1 ;;
esac

if [ -z "$kernel_url" ]; then
  case "$arch" in
    x86_64) kernel_file="vmlinux-5.10.bin" ;;
    aarch64) kernel_file="vmlinux-docker-5.10.bin" ;;
    *) echo "unsupported architecture for kernel: $arch" >&2; exit 1 ;;
  esac
  kernel_url="https://s3.amazonaws.com/spec.ccfc.min/img/quickstart_guide/${{arch}}/kernels/${{kernel_file}}"
fi

mkdir -p "{firecracker_runtime_dir}"
expected_source="url:$kernel_url"
if [ -s "$kernel_path" ] && [ -f "$kernel_source_marker" ]; then
  current_source="$(cat "$kernel_source_marker" 2>/dev/null || true)"
  if [ "$current_source" = "$expected_source" ]; then
    if [ -f "$kernel_sha_marker" ]; then
      current_sha="$(cat "$kernel_sha_marker" 2>/dev/null || true)"
      computed_sha="$(sha256sum "$kernel_path" | awk '{{print $1}}')"
      if [ -n "$current_sha" ] && [ "$current_sha" = "$computed_sha" ]; then
        exit 0
      fi
    else
      computed_sha="$(sha256sum "$kernel_path" | awk '{{print $1}}')"
      echo "$computed_sha" > "$kernel_sha_marker"
      exit 0
    fi
  fi
fi

tmp="$(mktemp -d)"
curl -fsSL -o "$tmp/vmlinux" "$kernel_url"
install -m 0644 "$tmp/vmlinux" "$kernel_path"
rm -rf "$tmp"
computed_sha="$(sha256sum "$kernel_path" | awk '{{print $1}}')"
echo "$expected_source" > "$kernel_source_marker"
echo "$computed_sha" > "$kernel_sha_marker"
"""
    ],
)

server.shell(
    name="Download Firecracker rootfs (if configured)",
    commands=[
        f"""
set -eu
rootfs_url="{firecracker_rootfs_url}"
rootfs_path="{firecracker_rootfs_path}"
rootfs_source_marker="{firecracker_rootfs_marker}"
rootfs_sha_marker="{firecracker_rootfs_sha_marker}"
if [ -z "$rootfs_url" ]; then
  exit 0
fi

mkdir -p "{firecracker_runtime_dir}"
expected_source="url:$rootfs_url"
if [ -s "$rootfs_path" ] && [ -f "$rootfs_source_marker" ]; then
  current_source="$(cat "$rootfs_source_marker" 2>/dev/null || true)"
  if [ "$current_source" = "$expected_source" ]; then
    if [ -f "$rootfs_sha_marker" ]; then
      current_sha="$(cat "$rootfs_sha_marker" 2>/dev/null || true)"
      computed_sha="$(sha256sum "$rootfs_path" | awk '{{print $1}}')"
      if [ -n "$current_sha" ] && [ "$current_sha" = "$computed_sha" ]; then
        exit 0
      fi
    else
      computed_sha="$(sha256sum "$rootfs_path" | awk '{{print $1}}')"
      echo "$computed_sha" > "$rootfs_sha_marker"
      exit 0
    fi
  fi
fi

tmp="$(mktemp -d)"
curl -fsSL -o "$tmp/rootfs.img" "$rootfs_url"
install -m 0644 "$tmp/rootfs.img" "$rootfs_path"
rm -rf "$tmp"
computed_sha="$(sha256sum "$rootfs_path" | awk '{{print $1}}')"
echo "$expected_source" > "$rootfs_source_marker"
echo "$computed_sha" > "$rootfs_sha_marker"
"""
    ],
)

server.shell(
    name="Initialize devmapper pool",
    commands=[
        f"""
set -eu
pool="{devmapper_pool}"
root_dir="{devmapper_dir}"
data_size="{devmapper_data_size}"
meta_size="{devmapper_meta_size}"
udev_enabled="{1 if devmapper_udev else 0}"

if ! command -v dmsetup >/dev/null 2>&1; then
  echo "dmsetup not available"
  exit 1
fi

mkdir -p "$root_dir"
data_file="$root_dir/data-device"
meta_file="$root_dir/meta-device"

if [ ! -f "$data_file" ]; then
  truncate -s "$data_size" "$data_file"
fi
if [ ! -f "$meta_file" ]; then
  truncate -s "$meta_size" "$meta_file"
fi

data_dev="$(losetup --output NAME --noheadings --associated "$data_file" 2>/dev/null || true)"
if [ -z "$data_dev" ]; then
  data_dev="$(losetup --find --show "$data_file")"
fi

meta_dev="$(losetup --output NAME --noheadings --associated "$meta_file" 2>/dev/null || true)"
if [ -z "$meta_dev" ]; then
  meta_dev="$(losetup --find --show "$meta_file")"
fi

sector_size=512
data_size_bytes="$(blockdev --getsize64 -q "$data_dev")"
length_sectors=$((data_size_bytes / sector_size))
data_block_size=128
low_water_mark=32768
table="0 ${{length_sectors}} thin-pool ${{meta_dev}} ${{data_dev}} ${{data_block_size}} ${{low_water_mark}} 1 skip_block_zeroing"

dm_env=""
dm_args=""
if [ "$udev_enabled" != "1" ]; then
  dm_env="DM_UDEV_DISABLE=1 DM_UDEV_DISABLE_DISK_RULES_FLAG=1"
  dm_args="--noudevsync --noudevrules"
fi

if env $dm_env dmsetup status "$pool" >/dev/null 2>&1; then
  env $dm_env dmsetup reload $dm_args "$pool" --table "$table"
else
  env $dm_env dmsetup create $dm_args "$pool" --table "$table"
fi
env $dm_env dmsetup mknodes "$pool" >/dev/null 2>&1 || true
"""
    ],
)

for module in ["kvm", "vhost_vsock", "tun"]:
    server.modprobe(
        name=f"Load kernel module {module}",
        module=module,
    )

server.service(
    name="Ensure Docker is enabled and running",
    service="docker",
    enabled=True,
    running=True,
)

server.shell(
    name="Build Firecracker rootfs image",
    commands=[
        f"""
set -eu
rootfs="{firecracker_rootfs_path}"
rootfs_url="{firecracker_rootfs_url}"
rootfs_source_marker="{firecracker_rootfs_marker}"
rootfs_sha_marker="{firecracker_rootfs_sha_marker}"
src="{firecracker_source_dir}"
ref="{firecracker_containerd_ref}"
run_as=""
if [ "{user}" != "root" ]; then
  run_as="sudo -u {user} -H"
fi

if [ -n "$rootfs_url" ]; then
  exit 0
fi

if [ -s "$rootfs" ]; then
  rootfs_type="$(blkid -o value -s TYPE "$rootfs" 2>/dev/null || true)"
  expected_source="build:$ref"
  if [ "$rootfs_type" = "squashfs" ] && [ -f "$rootfs_source_marker" ]; then
    current_source="$(cat "$rootfs_source_marker" 2>/dev/null || true)"
    if [ "$current_source" = "$expected_source" ]; then
      if [ -f "$rootfs_sha_marker" ]; then
        current_sha="$(cat "$rootfs_sha_marker" 2>/dev/null || true)"
        computed_sha="$(sha256sum "$rootfs" | awk '{{print $1}}')"
        if [ -n "$current_sha" ] && [ "$current_sha" = "$computed_sha" ]; then
          exit 0
        fi
      else
        computed_sha="$(sha256sum "$rootfs" | awk '{{print $1}}')"
        echo "$computed_sha" > "$rootfs_sha_marker"
        exit 0
      fi
    fi
  fi
fi

if [ ! -d "$src/.git" ]; then
  if [ "{user}" != "root" ]; then
    $run_as git clone https://github.com/firecracker-microvm/firecracker-containerd.git "$src"
  else
    git clone https://github.com/firecracker-microvm/firecracker-containerd.git "$src"
  fi
fi

if [ "{user}" = "root" ]; then
  git config --global --add safe.directory "$src" >/dev/null 2>&1 || true
fi

$run_as git -C "$src" fetch --depth 1 origin "$ref"
$run_as git -C "$src" checkout FETCH_HEAD

if command -v systemctl >/dev/null 2>&1; then
  systemctl start docker || true
fi
docker info >/dev/null

$run_as make -C "$src" image
install -m 0644 "$src/tools/image-builder/rootfs.img" "$rootfs"
expected_source="build:$ref"
computed_sha="$(sha256sum "$rootfs" | awk '{{print $1}}')"
echo "$expected_source" > "$rootfs_source_marker"
echo "$computed_sha" > "$rootfs_sha_marker"
"""
    ],
)

server.service(
    name="Ensure containerd is enabled and running",
    service="containerd",
    enabled=True,
    running=True,
)
