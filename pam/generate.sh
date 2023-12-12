#!/usr/bin/env sh

set -ex

PROEJECT_ROOT=$PWD/..
module_libname=pam_authd.so
loader_libname=pam_go_loader.so

if [ -d "$PROEJECT_ROOT"/vendor ]; then
    echo Vendored dependencies detected, not re-generating pam_module.go
else
    go run github.com/msteinert/pam/v2/cmd/pam-moduler \
        -libname "$module_libname" -type pamModule \
        "${@}"
fi

${CC:-cc} -o go-loader/"$loader_libname" \
    go-loader/module.c -Wl,--as-needed -Wl,--allow-shlib-undefined \
    -shared -fPIC -Wl,--unresolved-symbols=report-all \
    -Wl,-soname,"$loader_libname" -lpam

chmod 644 go-loader/"$loader_libname"
