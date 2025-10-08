#!/usr/bin/env bash

set -euo pipefail

if [ -v DEB_HOST_GNU_TYPE ]; then
    echo "Proto files should not be regenerated during package building"
    exit 0
fi

# TODO: Watch https://github.com/protocolbuffers/protobuf for any changes on the
# experimental status of optional fields, previously described on:
#  https://github.com/protocolbuffers/protobuf/blob/main/docs/implementing_proto3_presence.md.
args=(
    --proto_path=.
    --go_out=.
    --go_opt=paths=source_relative

    # Should it become default, remove the --experimental_allow_proto3_optional
    # flag from the go generate command below.
    --experimental_allow_proto3_optional
)

while [ "$#" -gt 0 ]; do
    case "$1" in
        --with-grpc)
            args+=(
                --go-grpc_out=.
                --go-grpc_opt=paths=source_relative
            )
            shift
        ;;
        --)
            shift
            break
        ;;
        -*)
            args+=("$1")
            shift
        ;;
        *)
            proto_file="$1"
            shift
        ;;
    esac
done

if [ ! -e "$proto_file" ]; then
    echo "No proto or invalid file provided: $proto_file"
    exit 1
fi

TOOLS_DIR=$(readlink -f "$(dirname "$0")")

protoc_gen_go_dir() {
    dirname "$(go -C "${TOOLS_DIR}" tool -n google.golang.org/protobuf/cmd/protoc-gen-go)"
}

protoc_gen_go_grpc_dir() {
    dirname "$(go -C "${TOOLS_DIR}" tool -n google.golang.org/grpc/cmd/protoc-gen-go-grpc)"
}

# In Go 1.24, `go tool -n` is affected by https://github.com/golang/go/issues/72824
# which makes it print a path like
#
#      /tmp/go-build1254405993/b001/exe/protoc-gen-go
#
# instead of
#
#     ~/.cache/go-build/1f/1f43080884166a56f6a9be495a3bc501b7d6dad6482461397d4bf946de142f6c-d/protoc-gen-go
#
# if the binary was not built yet. The workaround is to call `go tool -n` twice,
# the first time to build the binary, and the second time to get the correct path.
protoc_gen_go_dir >/dev/null 2>&1
PATH="$(protoc_gen_go_dir):$PATH"

protoc_gen_go_grpc_dir >/dev/null 2>&1
PATH="$(protoc_gen_go_grpc_dir):$PATH"

PATH=$PATH \
  exec protoc "${args[@]}" "$proto_file" "${@}"
