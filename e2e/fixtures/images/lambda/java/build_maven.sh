#!/usr/bin/env bash
# Where: e2e/fixtures/images/lambda/java/build_maven.sh
# What: Build helper for Java fixture Maven package with optional proxy settings.xml.
# Why: Maven does not reliably honor proxy env vars in all build contexts.
set -euo pipefail

proxy_raw="${HTTPS_PROXY:-${https_proxy:-${HTTP_PROXY:-${http_proxy:-}}}}"
no_proxy_raw="${NO_PROXY:-${no_proxy:-}}"

trim() {
  local value="$1"
  value="${value#"${value%%[![:space:]]*}"}"
  value="${value%"${value##*[![:space:]]}"}"
  printf '%s' "${value}"
}

xml_escape() {
  local value="$1"
  value="${value//&/&amp;}"
  value="${value//</&lt;}"
  value="${value//>/&gt;}"
  value="${value//\"/&quot;}"
  value="${value//\'/&apos;}"
  printf '%s' "${value}"
}

normalize_no_proxy_token() {
  local token
  token="$(trim "${1}")"
  if [[ -z "${token}" ]]; then
    return 0
  fi

  if [[ "${token}" == \[*\]* ]]; then
    token="${token#\[}"
    token="${token%%]*}"
  elif [[ "${token}" == *:* && "${token}" != *:*:* ]]; then
    local host="${token%:*}"
    local port="${token##*:}"
    if [[ "${port}" =~ ^[0-9]+$ ]]; then
      token="${host}"
    fi
  fi

  if [[ "${token}" == .* && "${token}" != \*.* ]]; then
    token="*${token}"
  fi

  printf '%s' "${token}"
}

build_non_proxy_hosts() {
  local raw="${1//;/,}"
  local -a tokens=()
  local token normalized
  IFS=',' read -r -a tokens <<< "${raw}"
  local output=""
  for token in "${tokens[@]}"; do
    normalized="$(normalize_no_proxy_token "${token}")"
    if [[ -z "${normalized}" ]]; then
      continue
    fi
    if [[ -z "${output}" ]]; then
      output="${normalized}"
    else
      output="${output}|${normalized}"
    fi
  done
  printf '%s' "${output}"
}

render_settings_xml() {
  local proxy_url
  proxy_url="$(trim "${1}")"
  local no_proxy_hosts
  no_proxy_hosts="$(build_non_proxy_hosts "$(trim "${2}")")"
  local settings_file="$3"

  if [[ -z "${proxy_url}" ]]; then
    return 1
  fi

  local scheme="${proxy_url%%://*}"
  local authority_and_path="${proxy_url#*://}"
  local authority="${authority_and_path%%/*}"
  if [[ -z "${scheme}" || -z "${authority}" ]]; then
    echo "invalid proxy URL: ${proxy_url}" >&2
    return 1
  fi

  scheme="${scheme,,}"
  if [[ "${scheme}" != "http" && "${scheme}" != "https" ]]; then
    echo "unsupported proxy URL scheme: ${proxy_url}" >&2
    return 1
  fi

  local userinfo=""
  local hostport="${authority}"
  if [[ "${authority}" == *"@"* ]]; then
    userinfo="${authority%@*}"
    hostport="${authority##*@}"
  fi

  local host=""
  local port=""
  if [[ "${hostport}" == \[*\]* ]]; then
    host="${hostport#\[}"
    host="${host%%]*}"
    local remainder="${hostport#*]}"
    if [[ "${remainder}" == :* ]]; then
      port="${remainder#:}"
    fi
  elif [[ "${hostport}" == *:* ]]; then
    host="${hostport%:*}"
    port="${hostport##*:}"
  else
    host="${hostport}"
  fi

  host="$(trim "${host}")"
  port="$(trim "${port}")"
  if [[ -z "${host}" ]]; then
    echo "proxy host is missing: ${proxy_url}" >&2
    return 1
  fi
  if [[ -n "${port}" && ! "${port}" =~ ^[0-9]+$ ]]; then
    echo "invalid proxy port in URL: ${proxy_url}" >&2
    return 1
  fi
  if [[ -z "${port}" ]]; then
    if [[ "${scheme}" == "https" ]]; then
      port="443"
    else
      port="80"
    fi
  fi

  local username=""
  local password=""
  if [[ -n "${userinfo}" ]]; then
    username="${userinfo%%:*}"
    if [[ "${userinfo}" == *":"* ]]; then
      password="${userinfo#*:}"
    fi
  fi

  {
    echo "<settings>"
    echo "  <proxies>"
    for protocol in http https; do
      echo "    <proxy>"
      echo "      <id>${protocol}-proxy</id>"
      echo "      <active>true</active>"
      echo "      <protocol>${protocol}</protocol>"
      echo "      <host>$(xml_escape "${host}")</host>"
      echo "      <port>${port}</port>"
      if [[ -n "${username}" ]]; then
        echo "      <username>$(xml_escape "${username}")</username>"
      fi
      if [[ -n "${password}" ]]; then
        echo "      <password>$(xml_escape "${password}")</password>"
      fi
      if [[ -n "${no_proxy_hosts}" ]]; then
        echo "      <nonProxyHosts>$(xml_escape "${no_proxy_hosts}")</nonProxyHosts>"
      fi
      echo "    </proxy>"
    done
    echo "  </proxies>"
    echo "</settings>"
  } > "${settings_file}"
}

build_with_maven() {
  local settings_path="/tmp/maven-proxy-settings.xml"
  if render_settings_xml "${proxy_raw}" "${no_proxy_raw}" "${settings_path}"; then
    mvn -s "${settings_path}" -q -DskipTests package
    return
  fi
  mvn -q -DskipTests package
}

build_with_maven
