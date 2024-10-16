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

tags=()

while [ "$#" -gt 0 ]; do
    case "$1" in
        --with-grpc)
            args+=(
                --go-grpc_out=.
                --go-grpc_opt=paths=source_relative
            )
            shift
        ;;
        --with-build-tag)
            tags+=("$2")
            shift 2
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

PATH="$(go env GOPATH)/bin:$PATH"
export PATH

if ! protoc "${args[@]}" "$proto_file" "${@}"; then
    exit $?
fi

base_file=$(basename "$proto_file" .proto)

for tag in "${tags[@]}"; do
    sed -i -e "1i //go:build $tag\n" "${base_file}.pb.go"

    if [ -e "${base_file}_grpc.pb.go" ]; then
        sed -i -e "1i //go:build $tag\n" "${base_file}_grpc.pb.go"
    fi
done
