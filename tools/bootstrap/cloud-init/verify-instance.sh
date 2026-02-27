#!/usr/bin/env bash
# Where: tools/bootstrap/cloud-init/verify-instance.sh
# What: Runs smoke tests inside a provisioned instance.
# Why: Verify cloud-init completion and runtime tool availability.
#
# Validation contract (source of truth):
# - cloud-init reaches "status: done" when available
# - /etc/bootstrap.env and /var/log/cloud-init-bootstrap.log exist
# - Docker CLI, compose plugin, and buildx plugin are usable
# - docker-ce and docker-ce-cli are installed
# - DOCKER_VERSION=latest skips minimum-version checks
# - DOCKER_VERSION=<value> enforces package version >= requested minimum
# - hello-world container run succeeds unless --skip-hello-world is passed
# - BOOTSTRAP_USER belongs to docker group when that user exists
# - mise command is available
# - gh command is available
# - SSL_INSPECTION_CA_CONFIGURED flag is present and reported
# - (optional) Hyper-V access settings are validated when expected values are passed:
#   - SSH PasswordAuthentication setting
#   - UFW active state and allowed inbound TCP ports
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage:
  sudo /tmp/verify-instance.sh [--bootstrap-user <user>] [--skip-hello-world] [--allow-cloud-init-disabled] [--expect-ssh-password-auth <enabled|disabled>] [--expect-open-tcp-ports <ports>]
USAGE
}

bootstrap_user="ubuntu"
skip_hello_world="false"
allow_cloud_init_disabled="false"
expect_ssh_password_auth=""
expect_open_tcp_ports=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --bootstrap-user)
      bootstrap_user="${2:-}"
      shift 2
      ;;
    --skip-hello-world)
      skip_hello_world="true"
      shift
      ;;
    --allow-cloud-init-disabled)
      allow_cloud_init_disabled="true"
      shift
      ;;
    --expect-ssh-password-auth)
      expect_ssh_password_auth="${2:-}"
      shift 2
      ;;
    --expect-open-tcp-ports)
      expect_open_tcp_ports="${2:-}"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown argument: $1" >&2
      usage >&2
      exit 1
      ;;
  esac
done

if [[ -n "${expect_ssh_password_auth}" ]]; then
  expect_ssh_password_auth="$(printf '%s' "${expect_ssh_password_auth}" | tr '[:upper:]' '[:lower:]')"
  if [[ "${expect_ssh_password_auth}" != "enabled" && "${expect_ssh_password_auth}" != "disabled" ]]; then
    echo "Invalid --expect-ssh-password-auth value: ${expect_ssh_password_auth}" >&2
    usage >&2
    exit 1
  fi
fi

log() {
  printf '[verify-instance] %s\n' "$*"
}

fail() {
  printf '[verify-instance] ERROR: %s\n' "$*" >&2
  exit 1
}

assert_file() {
  local path="$1"
  [[ -f "${path}" ]] || fail "missing file: ${path}"
}

installed_package_version() {
  local package="$1"
  local status_line=""
  status_line="$(dpkg-query -W -f='${db:Status-Abbrev} ${Version}\n' "${package}" 2>/dev/null || true)"
  if [[ "${status_line}" == ii\ * ]]; then
    printf '%s' "${status_line#ii }"
    return 0
  fi
  return 1
}

assert_package_minimum() {
  local package="$1"
  local minimum="$2"
  local current=""

  current="$(installed_package_version "${package}" || true)"
  if [[ -z "${current}" ]]; then
    fail "package not installed: ${package}"
  fi
  if ! dpkg --compare-versions "${current}" ge "${minimum}"; then
    fail "package ${package} version ${current} is below minimum ${minimum}"
  fi
}

log "wait cloud-init completion"
if command -v cloud-init >/dev/null 2>&1; then
  if [[ "${allow_cloud_init_disabled}" == "true" ]]; then
    touch /root/.cloud-warnings.skip 2>/dev/null || true
    mkdir -p /var/lib/cloud/instance/warnings 2>/dev/null || true
    touch /var/lib/cloud/instance/warnings/.skip 2>/dev/null || true
  fi

  cloud_init_status="$(cloud-init status --long 2>/dev/null || true)"
  if printf '%s\n' "${cloud_init_status}" | grep -Eq '^status:[[:space:]]+done$'; then
    :
  elif [[ "${allow_cloud_init_disabled}" == "true" ]] && printf '%s\n' "${cloud_init_status}" | grep -Eq '^status:[[:space:]]+disabled$'; then
    log "cloud-init status is disabled; accepted by --allow-cloud-init-disabled"
  else
    printf '%s\n' "${cloud_init_status}" >&2
    fail "cloud-init did not reach done state"
  fi
fi

log "check bootstrap artifacts"
assert_file /etc/bootstrap.env
assert_file /var/log/cloud-init-bootstrap.log

# shellcheck disable=SC1091
. /etc/bootstrap.env

if [[ -z "${SSL_INSPECTION_CA_CONFIGURED:-}" ]]; then
  fail "SSL_INSPECTION_CA_CONFIGURED is missing in /etc/bootstrap.env"
fi

docker_minimum="${DOCKER_VERSION:-latest}"
if [[ -z "${docker_minimum}" ]]; then
  docker_minimum="latest"
fi
docker_minimum_lower="$(printf '%s' "${docker_minimum}" | tr '[:upper:]' '[:lower:]')"

log "check docker commands"
command -v docker >/dev/null 2>&1 || fail "docker command not found"

docker version >/dev/null
docker compose version >/dev/null
docker buildx version >/dev/null

docker_ce_version="$(installed_package_version docker-ce || true)"
docker_cli_version="$(installed_package_version docker-ce-cli || true)"
if [[ -z "${docker_ce_version}" || -z "${docker_cli_version}" ]]; then
  fail "docker-ce / docker-ce-cli package not installed"
fi

if [[ "${docker_minimum_lower}" == "latest" ]]; then
  log "DOCKER_VERSION=latest; minimum version check skipped (docker-ce=${docker_ce_version}, docker-ce-cli=${docker_cli_version})"
else
  log "check minimum docker package versions"
  assert_package_minimum docker-ce "${docker_minimum}"
  assert_package_minimum docker-ce-cli "${docker_minimum}"
fi

if [[ "${skip_hello_world}" != "true" ]]; then
  log "run hello-world"
  docker run --rm hello-world >/dev/null
fi

if id "${bootstrap_user}" >/dev/null 2>&1; then
  if ! id -nG "${bootstrap_user}" | tr ' ' '\n' | grep -Fxq docker; then
    fail "${bootstrap_user} is not in docker group"
  fi
fi

log "check tooling"
command -v mise >/dev/null 2>&1 || fail "mise command not found"
mise --version >/dev/null
command -v gh >/dev/null 2>&1 || fail "gh command not found"
gh --version >/dev/null

if [[ "${SSL_INSPECTION_CA_CONFIGURED}" == "true" ]]; then
  log "custom CA configured"
else
  log "custom CA not configured"
fi

if [[ -n "${expect_ssh_password_auth}" ]]; then
  log "check ssh password authentication setting"
  ssh_override_conf="/etc/ssh/sshd_config.d/00-bootstrap-password-auth.conf"
  assert_file "${ssh_override_conf}"

  configured_password_auth="$(awk '
    /^[[:space:]]*PasswordAuthentication[[:space:]]+/ {
      print tolower($2)
      exit
    }' "${ssh_override_conf}")"
  if [[ -z "${configured_password_auth}" ]]; then
    fail "PasswordAuthentication entry not found in ${ssh_override_conf}"
  fi

  expected_password_auth="yes"
  if [[ "${expect_ssh_password_auth}" == "disabled" ]]; then
    expected_password_auth="no"
  fi

  if [[ "${configured_password_auth}" != "${expected_password_auth}" ]]; then
    fail "PasswordAuthentication mismatch: expected=${expected_password_auth}, actual=${configured_password_auth}"
  fi

  command -v sshd >/dev/null 2>&1 || fail "sshd command not found"
  sshd -t >/dev/null 2>&1 || fail "sshd config test failed"
  if ! sshd_t_output="$(sshd -T -C user=root -C host=localhost -C addr=127.0.0.1 2>&1)"; then
    fail "sshd -T failed: ${sshd_t_output}"
  fi
  effective_password_auth="$(printf '%s\n' "${sshd_t_output}" | awk '/^passwordauthentication /{print tolower($2); exit}')"
  if [[ -z "${effective_password_auth}" ]]; then
    fail "failed to evaluate effective sshd PasswordAuthentication setting"
  fi
  if [[ "${effective_password_auth}" != "${expected_password_auth}" ]]; then
    fail "effective sshd PasswordAuthentication mismatch: expected=${expected_password_auth}, actual=${effective_password_auth}"
  fi

  if command -v systemctl >/dev/null 2>&1 && [[ -d /run/systemd/system ]]; then
    systemctl is-active --quiet ssh || fail "ssh service is not active"
  fi
fi

if [[ -n "${expect_open_tcp_ports}" ]]; then
  log "check ufw allowed inbound tcp ports"
  command -v ufw >/dev/null 2>&1 || fail "ufw command not found"
  ufw_status="$(ufw status 2>/dev/null || true)"
  if ! printf '%s\n' "${ufw_status}" | grep -Eq '^Status:[[:space:]]+active$'; then
    fail "ufw is not active"
  fi

  normalized_ports="$(printf '%s' "${expect_open_tcp_ports}" | tr ',;/' ' ')"
  for token in ${normalized_ports}; do
    if [[ -z "${token}" ]]; then
      continue
    fi
    if ! [[ "${token}" =~ ^[0-9]+$ ]]; then
      fail "non-numeric TCP port in expected list: ${token}"
    fi
    if (( token < 1 || token > 65535 )); then
      fail "out-of-range TCP port in expected list: ${token}"
    fi
    if ! printf '%s\n' "${ufw_status}" | grep -Eq "(^|[[:space:]])${token}/tcp([[:space:]]|$)"; then
      fail "ufw is missing expected TCP port: ${token}"
    fi
  done
fi

log "OK"
