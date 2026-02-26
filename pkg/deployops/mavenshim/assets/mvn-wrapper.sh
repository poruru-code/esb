#!/usr/bin/env bash
# Where: pkg/deployops/mavenshim/assets/mvn-wrapper.sh
# What: Maven wrapper that applies proxy settings via settings.xml with JVM fallback.
# Why: Keep Maven proxy behavior compatible across real proxies and proxy.py-based E2E.
set -euo pipefail

script_path="$0"
if command -v readlink >/dev/null 2>&1; then
  script_path="$(readlink -f -- "$0" 2>/dev/null || printf '%s' "$0")"
fi

MAVEN_REAL_BIN="${MAVEN_REAL_BIN:-}"
if [[ -z "$MAVEN_REAL_BIN" && -x "${script_path}.esb-real" ]]; then
  MAVEN_REAL_BIN="${script_path}.esb-real"
fi
if [[ -z "$MAVEN_REAL_BIN" ]]; then
  MAVEN_REAL_BIN="/usr/share/maven/bin/mvn"
fi
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
  local regex='^([Hh][Tt][Tt][Pp][Ss]?)://([^/@[:space:]]+(:[^@[:space:]]*)?@)?(\[[^]]+\]|[^/:?#[:space:]]+)(:([0-9]+))?/?$'

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
http_host=""
http_port=""
http_username=""
http_password=""
https_host=""
https_port=""
https_username=""
https_password=""
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

if [[ -n "$http_endpoint" ]]; then
  IFS=$'\t' read -r http_host http_port http_username http_password <<< "$http_endpoint"
fi
if [[ -n "$https_endpoint" ]]; then
  IFS=$'\t' read -r https_host https_port https_username https_password <<< "$https_endpoint"
fi

non_proxy_hosts="$(build_non_proxy_hosts "$no_proxy")"
java_proxy_args=()

if [[ -n "$http_host" ]]; then
  java_proxy_args+=("-Dhttp.proxyHost=${http_host}")
  java_proxy_args+=("-Dhttp.proxyPort=${http_port}")
  if [[ -n "$http_username" ]]; then
    java_proxy_args+=("-Dhttp.proxyUser=${http_username}")
  fi
  if [[ -n "$http_password" ]]; then
    java_proxy_args+=("-Dhttp.proxyPassword=${http_password}")
  fi
fi

if [[ -n "$https_host" ]]; then
  java_proxy_args+=("-Dhttps.proxyHost=${https_host}")
  java_proxy_args+=("-Dhttps.proxyPort=${https_port}")
  if [[ -n "$https_username" ]]; then
    java_proxy_args+=("-Dhttps.proxyUser=${https_username}")
  fi
  if [[ -n "$https_password" ]]; then
    java_proxy_args+=("-Dhttps.proxyPassword=${https_password}")
  fi
fi

if [[ -n "$non_proxy_hosts" ]]; then
  if [[ -n "$http_host" ]]; then
    java_proxy_args+=("-Dhttp.nonProxyHosts=${non_proxy_hosts}")
  fi
  if [[ -n "$https_host" ]]; then
    java_proxy_args+=("-Dhttps.nonProxyHosts=${non_proxy_hosts}")
  fi
fi

settings_path="$(mktemp /tmp/esb-maven-settings-XXXXXX.xml)"
run_log_path="$(mktemp /tmp/esb-maven-wrapper-XXXXXX.log)"
trap 'rm -f "$settings_path" "$run_log_path"' EXIT
chmod 0600 "$settings_path"

{
  echo '<settings>'
  echo '  <proxies>'

  if [[ -n "$http_host" ]]; then
    echo '    <proxy>'
    echo '      <id>http-proxy</id>'
    echo '      <active>true</active>'
    echo '      <protocol>http</protocol>'
    echo "      <host>$(xml_escape "$http_host")</host>"
    echo "      <port>${http_port}</port>"
    if [[ -n "$http_username" ]]; then
      echo "      <username>$(xml_escape "$http_username")</username>"
    fi
    if [[ -n "$http_password" ]]; then
      echo "      <password>$(xml_escape "$http_password")</password>"
    fi
    if [[ -n "$non_proxy_hosts" ]]; then
      echo "      <nonProxyHosts>$(xml_escape "$non_proxy_hosts")</nonProxyHosts>"
    fi
    echo '    </proxy>'
  fi

  if [[ -n "$https_host" ]]; then
    echo '    <proxy>'
    echo '      <id>https-proxy</id>'
    echo '      <active>true</active>'
    echo '      <protocol>https</protocol>'
    echo "      <host>$(xml_escape "$https_host")</host>"
    echo "      <port>${https_port}</port>"
    if [[ -n "$https_username" ]]; then
      echo "      <username>$(xml_escape "$https_username")</username>"
    fi
    if [[ -n "$https_password" ]]; then
      echo "      <password>$(xml_escape "$https_password")</password>"
    fi
    if [[ -n "$non_proxy_hosts" ]]; then
      echo "      <nonProxyHosts>$(xml_escape "$non_proxy_hosts")</nonProxyHosts>"
    fi
    echo '    </proxy>'
  fi

  echo '  </proxies>'
  echo '</settings>'
} >"$settings_path"

set +e
"$MAVEN_REAL_BIN" -s "$settings_path" "$@" 2>&1 | tee "$run_log_path"
primary_rc=${PIPESTATUS[0]}
set -e

if [[ $primary_rc -eq 0 ]]; then
  exit 0
fi

if [[ ${#java_proxy_args[@]} -eq 0 ]]; then
  exit "$primary_rc"
fi

if ! grep -Eiq 'status code:[[:space:]]*407|proxy authentication required|proxyauthenticationfailed' "$run_log_path"; then
  exit "$primary_rc"
fi

echo "mvn shim: settings.xml proxy auth failed; retrying with JVM proxy properties" >&2
exec "$MAVEN_REAL_BIN" "${java_proxy_args[@]}" "$@"
