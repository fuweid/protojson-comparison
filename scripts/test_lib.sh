#!/usr/bin/env bash
# Copyright 2025 The etcd Authors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
# http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -euo pipefail

source ./scripts/test_utils.sh

ROOT_MODULE="github.com/fuweid/protojson-comparison"

if [[ "$(go list)" != "${ROOT_MODULE}" ]]; then
  echo "must be run from '${ROOT_MODULE}' module directory"
  exit 255
fi

function set_root_dir {
  ROOT_DIR=$(go list -f '{{.Dir}}' "${ROOT_MODULE}")
}

set_root_dir

####   Discovery of files/packages within a go module #####

# pkgs_in_module [optional:package_pattern]
# returns list of all packages in the current (dir) module.
# if the package_pattern is given, its being resolved.
function pkgs_in_module {
  go list -mod=mod "${1:-./...}";
}

# Prints subdirectory (from the repo root) for the current module.
function module_subdir {
  relativePath "${ROOT_DIR}" "${PWD}"
}

####    Running actions against multiple modules ####

# run [command...] - runs given command, printing it first and
# again if it failed (in RED). Use to wrap important test commands
# that user might want to re-execute to shorten the feedback loop when fixing
# the test.
function run {
  local rpath
  local command
  rpath=$(module_subdir)
  # Quoting all components as the commands are fully copy-parsable:
  command=("${@}")
  command=("${command[@]@Q}")
  if [[ "${rpath}" != "." && "${rpath}" != "" ]]; then
    repro="(cd ${rpath} && ${command[*]})"
  else 
    repro="${command[*]}"
  fi

  log_cmd "% ${repro}"
  "${@}" 2> >(while read -r line; do echo -e "${COLOR_NONE}stderr: ${COLOR_MAGENTA}${line}${COLOR_NONE}">&2; done)
  local error_code=$?
  if [ ${error_code} -ne 0 ]; then
    log_error -e "FAIL: (code:${error_code}):\\n  % ${repro}"
    return ${error_code}
  fi
}

# run_for_module [module] [cmd]
# executes given command in the given module for given pkgs.
#   module_name - "." (in future: tests, client, server)
#   cmd         - cmd to be executed - that takes package as last argument
function run_for_module {
  local module=${1:-"."}
  shift 1
  (
    cd "${ROOT_DIR}/${module}" && "$@"
  )
}

#### Other ####

# tool_get_bin [tool] - returns absolute path to a tool binary (or returns error).
# This function is only used to run commands that are managed by tools/mod.
function tool_get_bin {
  local tool="$1"
  local pkg_part="$1"
  if [[ "$tool" == *"@"* ]]; then
    pkg_part=$(echo "${tool}" | cut -d'@' -f1)
    # shellcheck disable=SC2086
    run go install ${GOBINARGS:-} "${tool}" || return 2
  else
    # shellcheck disable=SC2086
    run_for_module ./tools/mod run go install ${GOBINARGS:-} "${tool}" || return 2
  fi

  # remove the version suffix, such as removing "/v3" from "go.etcd.io/etcd/v3".
  local cmd_base_name
  cmd_base_name=$(basename "${pkg_part}")
  if [[ ${cmd_base_name} =~ ^v[0-9]*$ ]]; then
    pkg_part=$(dirname "${pkg_part}")
  fi

  run_for_module ./tools/mod go list -f '{{.Target}}' "${pkg_part}"
}

# tool_pkg_dir [pkg] - returns absolute path to a directory that stores given pkg.
# The pkg versions must be defined in ./tools/mod directory.
function tool_pkg_dir {
  run_for_module ./tools/mod run go list -f '{{.Dir}}' "${1}"
}

# tool_get_bin [tool]
function run_go_tool {
  local cmdbin
  if ! cmdbin=$(GOARCH="" GOOS="" tool_get_bin "${1}"); then
    log_warning "Failed to install tool '${1}'"
    return 2
  fi
  shift 1
  GOARCH="" run "${cmdbin}" "$@" || return 2
}

# The version present in the .go-verion is the default version that test and build scripts will use.
# However, it is possible to control the version that should be used with the help of env vars:
# - FORCE_HOST_GO: if set to a non-empty value, use the version of go installed in system's $PATH.
# - GO_VERSION: desired version of go to be used, might differ from what is present in .go-version.
#               If empty, the value defaults to the version in .go-version.
function determine_go_version {
  # Borrowing from how Kubernetes does this:
  #  https://github.com/kubernetes/kubernetes/blob/17854f0e0a153b06f9d0db096e2cd8ab2fa89c11/hack/lib/golang.sh#L510-L520
  #
  # default GO_VERSION to content of .go-version
  GO_VERSION="${GO_VERSION:-"$(cat "${ROOT_DIR}/.go-version")"}"
  if [ "${GOTOOLCHAIN:-auto}" != 'auto' ]; then
    # no-op, just respect GOTOOLCHAIN
    :
  elif [ -n "${FORCE_HOST_GO:-}" ]; then
    export GOTOOLCHAIN='local'
  else
    GOTOOLCHAIN="go${GO_VERSION}"
    export GOTOOLCHAIN
  fi
}

determine_go_version
