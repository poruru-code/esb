#!/usr/bin/env bash
# Where: tools/maven-shim/mvn-wrapper.sh
# What: Maven wrapper that injects proxy-aware settings.xml for every invocation.
# Why: Ensure Maven resolves dependencies behind authenticated proxies without per-Dockerfile hacks.
set -euo pipefail

MAVEN_REAL_BIN="${MAVEN_REAL_BIN:-/usr/share/maven/bin/mvn}"
if [[ ! -x "$MAVEN_REAL_BIN" ]]; then
  MAVEN_REAL_BIN="/usr/bin/mvn"
fi

trim() {
  local value="$1"
  value="${value#"${value%%[![:space:]]*}"}"
  value="${value%"${value##*[![:space:]]}"}"
  printf '%s' "$value"
}

resolve_env_alias() {
  local upper="$1"
  local lower="$2"
  local value
  value="$(trim "${!upper:-}")"
  if [[ -n "$value" ]]; then
    printf '%s' "$value"
    return
  fi
  value="$(trim "${!lower:-}")"
  printf '%s' "$value"
}

url_decode() {
  local value="$1"
  if [[ "$value" != *%* ]]; then
    printf '%s' "$value"
    return
  fi
  printf '%b' "${value//%/\\x}" 2>/dev/null || printf '%s' "$value"
}

xml_escape() {
  local value="$1"
  value="${value//&/&amp;}"
  value="${value//</&lt;}"
  value="${value//>/&gt;}"
  value="${value//\"/&quot;}"
  value="${value//\'/&apos;}"
  printf '%s' "$value"
}

parse_proxy_endpoint() {
  local raw="$1"
  local label="$2"
  local regex='^(http|https)://([^/@[:space:]]+(:[^@[:space:]]*)?@)?(\[[^]]+\]|[^/:?#[:space:]]+)(:([0-9]+))?/?$'

  if [[ ! "$raw" =~ $regex ]]; then
    echo "mvn shim: invalid proxy URL for ${label}: ${raw}" >&2
    return 1
  fi

  local scheme="${BASH_REMATCH[1]}"
  local userinfo="${BASH_REMATCH[2]}"
  local host="${BASH_REMATCH[4]}"
  local port="${BASH_REMATCH[6]}"
  local username=""
  local password=""

  if [[ "$host" == \[*\] ]]; then
    host="${host:1:${#host}-2}"
  fi

  if [[ -z "$port" ]]; then
    if [[ "${scheme,,}" == "https" ]]; then
      port="443"
    else
      port="80"
    fi
  fi

  if ! [[ "$port" =~ ^[0-9]+$ ]] || (( port < 1 || port > 65535 )); then
    echo "mvn shim: invalid proxy URL port for ${label}: ${raw}" >&2
    return 1
  fi

  if [[ -n "$userinfo" ]]; then
    userinfo="${userinfo%@}"
    if [[ "$userinfo" == *:* ]]; then
      username="${userinfo%%:*}"
      password="${userinfo#*:}"
    else
      username="$userinfo"
    fi
    username="$(url_decode "$username")"
    password="$(url_decode "$password")"
  fi

  printf '%s\t%s\t%s\t%s\n' "$host" "$port" "$username" "$password"
}

normalize_no_proxy_token() {
  local token
  token="$(trim "$1")"
  if [[ -z "$token" ]]; then
    return 1
  fi

  if [[ "$token" =~ ^\[([^]]+)\](:[0-9]+)?$ ]]; then
    token="${BASH_REMATCH[1]}"
  elif [[ "$token" =~ ^([^:]+):([0-9]+)$ ]]; then
    token="${BASH_REMATCH[1]}"
  fi

  if [[ "$token" == .* && "$token" != '*.'* ]]; then
    token="*${token}"
  fi

  printf '%s' "$token"
}

build_non_proxy_hosts() {
  local raw="$1"
  local replaced="${raw//;/,}"
  local token=""
  local joined=""
  local normalized=""
  declare -A seen=()

  IFS=',' read -ra tokens <<< "$replaced"
  for token in "${tokens[@]}"; do
    normalized="$(normalize_no_proxy_token "$token" || true)"
    if [[ -z "$normalized" ]]; then
      continue
    fi
    if [[ -n "${seen[$normalized]+x}" ]]; then
      continue
    fi
    seen["$normalized"]=1
    if [[ -z "$joined" ]]; then
      joined="$normalized"
    else
      joined+="|$normalized"
    fi
  done

  printf '%s' "$joined"
}

http_proxy="$(resolve_env_alias HTTP_PROXY http_proxy)"
https_proxy="$(resolve_env_alias HTTPS_PROXY https_proxy)"
no_proxy="$(resolve_env_alias NO_PROXY no_proxy)"

http_endpoint=""
https_endpoint=""
if [[ -n "$http_proxy" ]]; then
  http_endpoint="$(parse_proxy_endpoint "$http_proxy" "HTTP_PROXY/http_proxy")"
fi
if [[ -n "$https_proxy" ]]; then
  https_endpoint="$(parse_proxy_endpoint "$https_proxy" "HTTPS_PROXY/https_proxy")"
elif [[ -n "$http_endpoint" ]]; then
  https_endpoint="$http_endpoint"
fi

if [[ -z "$http_endpoint" && -z "$https_endpoint" ]]; then
  exec "$MAVEN_REAL_BIN" "$@"
fi

settings_path="$(mktemp /tmp/esb-maven-settings-XXXXXX.xml)"
trap 'rm -f "$settings_path"' EXIT
chmod 0600 "$settings_path"

non_proxy_hosts="$(build_non_proxy_hosts "$no_proxy")"

{
  echo '<settings>'
  echo '  <proxies>'

  if [[ -n "$http_endpoint" ]]; then
    IFS=$'\t' read -r host port username password <<< "$http_endpoint"
    echo '    <proxy>'
    echo '      <id>http-proxy</id>'
    echo '      <active>true</active>'
    echo '      <protocol>http</protocol>'
    echo "      <host>$(xml_escape "$host")</host>"
    echo "      <port>${port}</port>"
    if [[ -n "$username" ]]; then
      echo "      <username>$(xml_escape "$username")</username>"
    fi
    if [[ -n "$password" ]]; then
      echo "      <password>$(xml_escape "$password")</password>"
    fi
    if [[ -n "$non_proxy_hosts" ]]; then
      echo "      <nonProxyHosts>$(xml_escape "$non_proxy_hosts")</nonProxyHosts>"
    fi
    echo '    </proxy>'
  fi

  if [[ -n "$https_endpoint" ]]; then
    IFS=$'\t' read -r host port username password <<< "$https_endpoint"
    echo '    <proxy>'
    echo '      <id>https-proxy</id>'
    echo '      <active>true</active>'
    echo '      <protocol>https</protocol>'
    echo "      <host>$(xml_escape "$host")</host>"
    echo "      <port>${port}</port>"
    if [[ -n "$username" ]]; then
      echo "      <username>$(xml_escape "$username")</username>"
    fi
    if [[ -n "$password" ]]; then
      echo "      <password>$(xml_escape "$password")</password>"
    fi
    if [[ -n "$non_proxy_hosts" ]]; then
      echo "      <nonProxyHosts>$(xml_escape "$non_proxy_hosts")</nonProxyHosts>"
    fi
    echo '    </proxy>'
  fi

  echo '  </proxies>'
  echo '</settings>'
} >"$settings_path"

exec "$MAVEN_REAL_BIN" -s "$settings_path" "$@"
