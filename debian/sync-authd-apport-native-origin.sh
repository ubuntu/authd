#!/bin/sh
set -eu

if [ "$#" -gt 4 ]; then
    printf 'Usage: %s [sources-list [sources-list.d [native-origins.d [origin-file]]]]\n' "$0" >&2
    exit 2
fi

sources_list=${1:-/etc/apt/sources.list}
sources_list_dir=${2:-/etc/apt/sources.list.d}
native_origins_dir=${3:-/etc/apport/native-origins.d}
native_origin_file=${4:-/usr/lib/authd/authd-native-origins}
native_origin_entry="$native_origins_dir/authd"

list_file_has_authd_ppa() {
    awk '
        function is_authd_ppa(uri) {
            return uri ~ /^https?:\/\/ppa\.launchpad(content)?\.net\/ubuntu-enterprise-desktop\/authd\/ubuntu\/?$/
        }
        /^[[:space:]]*#/ {
            next
        }
        /^[[:space:]]*deb(-src)?[[:space:]]+/ {
            line = $0
            sub(/\r$/, "", line)
            sub(/^[[:space:]]*deb(-src)?[[:space:]]+/, "", line)
            if (substr(line, 1, 1) == "[") {
                options_end = index(line, "]")
                if (options_end > 0) {
                    line = substr(line, options_end + 1)
                }
            }
            sub(/^[[:space:]]+/, "", line)
            split(line, fields, /[[:space:]]+/)
            if (is_authd_ppa(fields[1])) {
                found = 1
            }
        }
        END {
            exit !found
        }
    ' "$1"
}

sources_file_has_authd_ppa() {
    awk '
        function is_authd_ppa(uri) {
            return uri ~ /^https?:\/\/ppa\.launchpad(content)?\.net\/ubuntu-enterprise-desktop\/authd\/ubuntu\/?$/
        }
        function scan_uris(value, uris, count, i) {
            count = split(value, uris, /[[:space:]]+/)
            for (i = 1; i <= count; i++) {
                if (is_authd_ppa(uris[i])) {
                    ppa = 1
                }
            }
        }
        function finish_stanza() {
            sub(/^[[:space:]]+/, "", enabled_value)
            sub(/[[:space:]]+$/, "", enabled_value)
            if (tolower(enabled_value) !~ /^(no|false|off|0)$/ && ppa) {
                found = 1
            }
            enabled_value = ""
            ppa = 0
            field = ""
        }
        BEGIN {
            RS = ""
            FS = "\n"
        }
        {
            enabled_value = ""
            ppa = 0
            field = ""
            for (i = 1; i <= NF; i++) {
                line = $i
                sub(/\r$/, "", line)
                if (line ~ /^[[:space:]]*$/) {
                    finish_stanza()
                    continue
                }
                if (line ~ /^[[:space:]]*#/) {
                    continue
                }
                if (line ~ /^[[:space:]]/) {
                    if (field == "uris") {
                        scan_uris(line)
                    } else if (field == "enabled") {
                        sub(/^[[:space:]]+/, "", line)
                        enabled_value = enabled_value " " line
                    }
                    continue
                }
                field = line
                sub(/:.*/, "", field)
                value = line
                sub(/^[^:]*:[[:space:]]*/, "", value)
                sub(/^[[:space:]]+/, "", value)
                sub(/[[:space:]]+$/, "", value)
                field = tolower(field)
                if (field == "enabled") {
                    enabled_value = value
                }
                if (field == "uris") {
                    scan_uris(value)
                }
            }
            finish_stanza()
        }
        END {
            exit !found
        }
    ' "$1"
}

authd_ppa_is_enabled() {
    if [ -f "$sources_list" ] && list_file_has_authd_ppa "$sources_list"; then
        return 0
    fi

    for source in "$sources_list_dir"/*.list "$sources_list_dir"/*.sources; do
        [ -f "$source" ] || continue
        case "${source##*/}" in
            *[!a-zA-Z0-9_.-]*) continue ;;
        esac
        case "$source" in
            *.list) list_file_has_authd_ppa "$source" && return 0 ;;
            *.sources) sources_file_has_authd_ppa "$source" && return 0 ;;
        esac
    done

    return 1
}

remove_native_origin() {
    if [ -L "$native_origin_entry" ] &&
       [ "$(readlink "$native_origin_entry")" = "$native_origin_file" ]; then
        rm -- "$native_origin_entry"
    elif [ -f "$native_origin_entry" ] && [ ! -L "$native_origin_entry" ] &&
         [ -f "$native_origin_file" ] &&
         cmp -s "$native_origin_entry" "$native_origin_file"; then
        rm -- "$native_origin_entry"
    fi
}

enable_native_origin() {
    if [ ! -f "$native_origin_file" ]; then
        printf 'authd: Apport native-origin file is missing: %s\n' "$native_origin_file" >&2
        return 1
    fi

    mkdir -p "$native_origins_dir"

    if [ -L "$native_origin_entry" ]; then
        if [ "$(readlink "$native_origin_entry")" = "$native_origin_file" ]; then
            return 0
        fi
        printf 'authd: not replacing existing Apport native-origin entry: %s\n' "$native_origin_entry" >&2
        return 0
    fi

    if [ -e "$native_origin_entry" ]; then
        if [ -f "$native_origin_entry" ] && cmp -s "$native_origin_entry" "$native_origin_file"; then
            return 0
        fi
        printf 'authd: not replacing existing Apport native-origin entry: %s\n' "$native_origin_entry" >&2
        return 0
    fi

    ln -s -- "$native_origin_file" "$native_origin_entry"
}

if authd_ppa_is_enabled; then
    enable_native_origin
else
    remove_native_origin
fi
