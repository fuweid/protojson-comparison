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
#
# Generate all etcd protobuf bindings.
# Run from repository root directory named etcd.
#
set -euo pipefail

shopt -s globstar

if ! [[ "$0" =~ scripts/genproto.sh ]]; then
  echo "must be run from repository root"
  exit 255
fi

if [ -z "${OS:-}" ]; then
  OS=$(uname -s | tr '[:upper:]' '[:lower:]')
fi

# Set SED variable
if LANG=C sed --help 2>&1 | grep -q GNU; then
  SED="sed"
else
  echo "Failed to find GNU sed as sed or gsed." >&2
  exit 1
fi

source ./scripts/test_lib.sh

PATH=$(pwd)/bin:$(go env GOPATH)/bin:$PATH
export PATH

if [[ $(protoc --version | cut -f2 -d' ') != "3.20.3" ]]; then
  echo "Could not find protoc 3.20.3, installing now..."

  arch=$(go env GOARCH)

  case ${arch} in
    "amd64") file="x86_64" ;;
    "arm64") file="aarch_64" ;;
    *)
      echo "Unsupported architecture: ${arch}"
      exit 255
      ;;
  esac

  protoc_download_file="protoc-3.20.3-linux-${file}.zip"
  if [ "$OS" == "darwin" ]; then
    # protoc-3.20.3 does not have pre-built binaries for darwin_arm64. Thanks to Rosetta, we could use x86_64 binary.
    protoc_download_file="protoc-3.20.3-osx-x86_64.zip"
  fi
  download_url="https://github.com/protocolbuffers/protobuf/releases/download/v3.20.3/${protoc_download_file}"
  echo "Running on ${OS} ${arch}. Downloading ${protoc_download_file}"
  mkdir -p bin
  wget ${download_url} && unzip -p ${protoc_download_file} bin/protoc > tmpFile && mv tmpFile bin/protoc
  rm ${protoc_download_file}
  chmod +x bin/protoc
  echo "Now running: $(protoc --version)"

fi

GOGEN_BIN=$(tool_get_bin google.golang.org/protobuf/cmd/protoc-gen-go)
GOGENGRPC_BIN=$(tool_get_bin google.golang.org/grpc/cmd/protoc-gen-go-grpc)
GRPC_GATEWAY_BIN=$(tool_get_bin github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway)
OPENAPIV2_BIN=$(tool_get_bin github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-openapiv2)
GOGOPROTO_ROOT="$(tool_pkg_dir github.com/gogo/protobuf/proto)/.."
GRPC_GATEWAY_ROOT="$(tool_pkg_dir github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway)/.."
GOOGLEAPI_ROOT=$(mktemp -d -t 'googleapi.XXXXX')

module_mapping_list=(
  Mgoogle/protobuf/descriptor.proto=google.golang.org/protobuf/types/descriptorpb
  Mgoogle/protobuf/struct.proto=google.golang.org/protobuf/types/known/structpb
)
module_mappings=$(IFS=$','; echo "${module_mapping_list[*]}" )

readonly googleapi_commit=0adf469dcd7822bf5bc058a7b0217f5558a75643

function cleanup_googleapi() {
  rm -rf "${GOOGLEAPI_ROOT}"
}

trap cleanup_googleapi EXIT

# TODO(ahrtr): use buf (https://github.com/bufbuild/buf) to manage the protobuf dependencies?
function download_googleapi() {
  run pushd "${GOOGLEAPI_ROOT}"
  run git init
  run git remote add upstream https://github.com/googleapis/googleapis.git
  run git fetch upstream "${googleapi_commit}"
  run git reset --hard FETCH_HEAD
  run popd
}

download_googleapi

echo
echo "Resolved binary and packages versions:"
echo "  - protoc-gen-go:           ${GOGEN_BIN}"
echo "  - protoc-gen-go-grpc:      ${GOGENGRPC_BIN}"
echo "  - protoc-gen-grpc-gateway: ${GRPC_GATEWAY_BIN}"
echo "  - openapiv2:               ${OPENAPIV2_BIN}"
echo "  - gogoproto-root:          ${GOGOPROTO_ROOT}"
echo "  - grpc-gateway-root:       ${GRPC_GATEWAY_ROOT}"
GOGOPROTO_PATH="${GOGOPROTO_ROOT}:${GOGOPROTO_ROOT}/protobuf"

# directories containing protos to be built
DIRS="./api/etcdserverpb ./api/mvccpb ./api/authpb ./api/membershippb ./api/versionpb"

log_callout -e "\\nRunning protoc-gen-go proto generation..."

for dir in ${DIRS}; do
  run pushd "${dir}"
    run protoc --go_out=. -I=".:${GOGOPROTO_PATH}:${ROOT_DIR}/..:${ROOT_DIR}:${GOOGLEAPI_ROOT}" \
      "--go_opt=paths=source_relative,${module_mappings}" \
      --go-grpc_out=. \
      "--go-grpc_opt=paths=source_relative,${module_mappings}" \
      -I"${GRPC_GATEWAY_ROOT}" \
      ./**/*.proto

    run gofmt -s -w ./**/*.pb.go
    run_go_tool "golang.org/x/tools/cmd/goimports" -w ./**/*.pb.go
  run popd
done
