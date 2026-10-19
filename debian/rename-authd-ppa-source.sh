#!/bin/sh
set -eu

if [ "$#" -gt 1 ]; then
    printf 'Usage: %s [sources-directory]\n' "$0" >&2
    exit 2
fi

sources_dir=${1:-/etc/apt/sources.list.d}
[ -d "$sources_dir" ] || exit 0

for source in "$sources_dir"/ubuntu-enterprise-desktop-ubuntu-authd-*.sources; do
    [ -f "$source" ] || continue

    if ! grep -Eq '^URIs:[[:space:]]*https?://ppa\.launchpad(content)?\.net/ubuntu-enterprise-desktop/authd/ubuntu/?([[:space:]]|$)' "$source"; then
        continue
    fi

    # Keep the filename stable; apt gets the suite from the source contents.
    target="$sources_dir/ubuntu-enterprise-desktop-ubuntu-authd.sources"

    if [ -e "$target" ] || [ -L "$target" ]; then
        printf 'authd: not renaming %s to %s: target already exists\n' "$source" "$target" >&2
        continue
    fi

    if ! mv -- "$source" "$target"; then
        printf 'authd: failed to rename %s to %s\n' "$source" "$target" >&2
    fi
done
