#!/bin/sh
set -eu

CARGO_HOME=${DEB_CARGO_HOME:-$(mktemp --tmpdir -d -t "cargo-home-XXXXXX")}
export CARGO_HOME
trap 'rm -rf "$CARGO_HOME"' EXIT INT HUP

# Some crates are shipped with .a files, which get removed by the helpers during the package build as a safety measure.
# This results in cargo failing to compile, since the files (which are listed in the checksums) are not there anymore.
# For those crates, we need to replace their checksum with a more general one that only lists the crate checksum, instead of each file.
${CARGO} vendor "${CARGO_VENDOR_DIR}"

[ ! -e "${DH_CARGO_VENDORED_SOURCES}" ] || ${DH_CARGO_VENDORED_SOURCES}
[ -e /usr/bin/jq ] || (echo "jq is required to run this script. Try installing it with 'sudo apt install jq'" && exit 1)

for dep in vendor_rust/*; do
    checksum_file="${dep}/.cargo-checksum.json"
    a_files=$(jq '.files | keys | map(select(.|test(".a$")))' "${checksum_file}")
    if [ "$a_files" = "[]" ]; then
        continue
    fi
    pkg_checksum=$(jq '.package' "${checksum_file}")
    echo "{\"files\": {}, \"package\": ${pkg_checksum}}" > "${checksum_file}"
done
