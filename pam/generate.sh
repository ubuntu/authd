#!/usr/bin/env bash

set -euo pipefail

SCRIPT_PATH=$(dirname "$0")
PROJECT_ROOT=$(realpath "$SCRIPT_PATH")/..
module_libname=pam_authd.so
loader_libname=pam_authd_loader.so
exec_libname=pam_authd_exec.so

cd "$SCRIPT_PATH"

if [ -d "$PROJECT_ROOT"/vendor ]; then
    echo Vendored dependencies detected, not re-generating pam_module.go
else
    go run github.com/msteinert/pam/v2/cmd/pam-moduler \
        -libname "$module_libname" -type pamModule -no-main \
        "${@}"
    go generate -x -tags pam_module_generation
fi

cc_args=()
if [ -v AUTHD_PAM_MODULES_PATH ]; then
    cc_args+=(-DAUTHD_PAM_MODULES_PATH=\""${AUTHD_PAM_MODULES_PATH}"\")
fi

# shellcheck disable=SC2086
# we do want to do word splitting on flags
${CC:-cc} -o go-loader/"$loader_libname" \
    go-loader/module.c ${CFLAGS:--Wall} -Wl,--as-needed -Wl,--allow-shlib-undefined \
    -shared -fPIC -Wl,--unresolved-symbols=report-all \
    -Wl,-soname,"$loader_libname" -lpam ${LDFLAGS:-} "${cc_args[@]}"

chmod 644 go-loader/"$loader_libname"

# shellcheck disable=SC2086
# we do want to do word splitting on flags
${CC:-cc} -o go-exec/"$exec_libname" \
    go-exec/module.c ${CFLAGS:--Wall} \
    $(pkg-config --cflags gio-2.0 gio-unix-2.0) \
    -Wl,--as-needed -Wl,--allow-shlib-undefined \
    -shared -fPIC -Wl,--unresolved-symbols=report-all \
    -Wl,-soname,"$exec_libname" \
    $(pkg-config --libs gio-2.0 gio-unix-2.0) \
    -lpam ${LDFLAGS:-} "${cc_args[@]}"

chmod 644 go-exec/"$exec_libname"
