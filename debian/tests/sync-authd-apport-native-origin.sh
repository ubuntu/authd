#!/bin/sh
set -eu

helper=${1:-/usr/lib/authd/sync-authd-apport-native-origin.sh}
origin_files_dir=${2:-/usr/lib/authd}
apt_hook=${3:-/etc/apt/apt.conf.d/99authd-apport-native-origin}
postinst=${4:-debian/postinst}
postrm=${5:-debian/postrm}
tmpdir=$(mktemp -d)
trap 'rm -rf "$tmpdir"' 0

sources_list="$tmpdir/sources.list"
sources_list_dir="$tmpdir/sources.list.d"
native_origins_dir="$tmpdir/native-origins.d"
mkdir -p "$sources_list_dir" "$native_origins_dir"

assert_enabled() {
    ppa=$1
    native_origin_entry="$native_origins_dir/$ppa"
    native_origin_file="$origin_files_dir/$ppa-native-origins"
    [ -L "$native_origin_entry" ]
    [ "$(readlink "$native_origin_entry")" = "$native_origin_file" ]
    [ "$(cat "$native_origin_file")" = "LP-PPA-ubuntu-enterprise-desktop-$ppa" ]
}

assert_disabled() {
    ppa=$1
    native_origin_entry="$native_origins_dir/$ppa"
    [ ! -e "$native_origin_entry" ]
    [ ! -L "$native_origin_entry" ]
}

assert_only_enabled() {
    expected=${1:-}
    for ppa in authd authd-candidate authd-edge authd-dev; do
        if [ "$ppa" = "$expected" ]; then
            assert_enabled "$ppa"
        else
            assert_disabled "$ppa"
        fi
    done
}

sync_origin() {
    "$helper" "$sources_list" "$sources_list_dir" "$native_origins_dir" "$origin_files_dir"
}

sync_origin
assert_only_enabled

cat > "$sources_list_dir/authd.sources" <<'EOF'
Types: deb
URIs: https://archive.ubuntu.com/ubuntu/
  https://ppa.launchpadcontent.net/ubuntu-enterprise-desktop/authd/ubuntu/
Suites: noble
Components: main
EOF
sync_origin
assert_only_enabled authd
sync_origin
assert_only_enabled authd
printf 'Types: deb\r\nURIs: https://ppa.launchpadcontent.net/ubuntu-enterprise-desktop/authd/ubuntu/\r\nSuites: noble\r\nComponents: main\r\n\r\nTypes: deb\r\nURIs: https://archive.ubuntu.com/ubuntu/\r\nSuites: noble\r\nComponents: main\r\nEnabled: no\r\n' > "$sources_list_dir/authd.sources"
sync_origin
assert_only_enabled authd

printf 'Types: deb\r\nURIs: https://archive.ubuntu.com/ubuntu/\r\nSuites: noble\r\nComponents: main\r\nEnabled: no\r\n\r\nTypes: deb\r\nURIs: https://ppa.launchpadcontent.net/ubuntu-enterprise-desktop/authd/ubuntu/\r\nSuites: noble\r\nComponents: main\r\n' > "$sources_list_dir/authd.sources"
sync_origin
assert_only_enabled authd

cat > "$sources_list_dir/candidate.sources" <<'EOF'
Types: deb
URIs: https://ppa.launchpadcontent.net/ubuntu-enterprise-desktop/authd-candidate/ubuntu/
Suites: noble
Components: main
EOF
sync_origin
assert_enabled authd
assert_enabled authd-candidate
assert_disabled authd-edge
assert_disabled authd-dev

rm "$sources_list_dir/candidate.sources"
sync_origin
assert_only_enabled authd

cat > "$sources_list_dir/authd.sources" <<'EOF'
Types: deb
URIs: https://ppa.launchpadcontent.net/ubuntu-enterprise-desktop/authd/ubuntu/
Suites: noble
Components: main
Enabled: no
EOF
sync_origin
assert_only_enabled

sed '/^Enabled:/d' "$sources_list_dir/authd.sources" > "$tmpdir/disabled.sources"
for disabled in no false off 0 NO False; do
    for separator in ' ' '\n '; do
        cp "$tmpdir/disabled.sources" "$sources_list_dir/authd.sources"
        printf 'Enabled:%b%s\n' "$separator" "$disabled" >> "$sources_list_dir/authd.sources"
        sync_origin
        assert_only_enabled
    done
done

cat > "$sources_list_dir/authd.sources" <<'EOF'
Types: deb
URIs: https://ppa.launchpadcontent.net/ubuntu-enterprise-desktop/authd-edge/ubuntu/
Suites: noble
Components: main
EOF
sync_origin
assert_only_enabled authd-edge

mv "$sources_list_dir/authd.sources" "$sources_list_dir/authd disabled.sources"
sync_origin
assert_only_enabled
mv "$sources_list_dir/authd disabled.sources" "$sources_list_dir/authd.sources"

rm "$sources_list_dir/authd.sources"
cat > "$sources_list_dir/candidate.sources" <<'EOF'
Types: deb
URIs: https://ppa.launchpadcontent.net/ubuntu-enterprise-desktop/authd-candidate/ubuntu/
Suites: noble
Components: main
EOF
sync_origin
assert_only_enabled authd-candidate

rm "$sources_list_dir/candidate.sources"
cat > "$sources_list_dir/authd.list" <<'EOF'
deb [arch=amd64] http://ppa.launchpad.net/ubuntu-enterprise-desktop/authd-edge/ubuntu/ noble main
EOF
sync_origin
assert_only_enabled authd-edge

mv "$sources_list_dir/authd.list" "$sources_list_dir/authd@disabled.list"
sync_origin
assert_only_enabled
mv "$sources_list_dir/authd@disabled.list" "$sources_list_dir/authd.list"

rm "$sources_list_dir/authd.list"
cat > "$sources_list" <<'EOF'
deb-src https://ppa.launchpad.net/ubuntu-enterprise-desktop/authd-dev/ubuntu noble main
EOF
sync_origin
assert_only_enabled authd-dev

: > "$sources_list"
sync_origin
assert_only_enabled

cp "$origin_files_dir/authd-native-origins" "$native_origins_dir/authd"
sync_origin
assert_only_enabled

cp "$origin_files_dir/authd-native-origins" "$tmpdir/admin-origin"
ln -s "$tmpdir/admin-origin" "$native_origins_dir/authd"
sync_origin
[ -L "$native_origins_dir/authd" ]
[ "$(readlink "$native_origins_dir/authd")" = "$tmpdir/admin-origin" ]
rm "$tmpdir/admin-origin"
sync_origin
[ -L "$native_origins_dir/authd" ]
[ "$(readlink "$native_origins_dir/authd")" = "$tmpdir/admin-origin" ]
rm "$native_origins_dir/authd"

cat > "$sources_list_dir/authd.sources" <<'EOF'
Types: deb
URIs: https://ppa.launchpadcontent.net/ubuntu-enterprise-desktop/authd/ubuntu/
Suites: noble
Components: main
EOF
printf 'custom origin\n' > "$native_origins_dir/authd"
sync_origin 2>"$tmpdir/error"
[ "$(cat "$native_origins_dir/authd")" = 'custom origin' ]
grep -q 'not replacing existing Apport native-origin entry' "$tmpdir/error"

rm "$native_origins_dir/authd"
cat > "$sources_list_dir/authd.sources" <<'EOF'
Types: deb
URIs: https://ppa.launchpadcontent.net/ubuntu-enterprise-desktop/authd/ubuntu/
  https://ppa.launchpadcontent.net/ubuntu-enterprise-desktop/authd-candidate/ubuntu/
  https://ppa.launchpadcontent.net/ubuntu-enterprise-desktop/authd-edge/ubuntu/
  https://ppa.launchpadcontent.net/ubuntu-enterprise-desktop/authd-dev/ubuntu/
Suites: noble
Components: main
EOF
sync_origin
for ppa in authd authd-candidate authd-edge authd-dev; do
    assert_enabled "$ppa"
done

hook_command=$(sed -n 's/^ *"\(.*\)";$/\1/p' "$apt_hook" |
    sed "s|/usr/lib/authd/sync-authd-apport-native-origin.sh|$tmpdir/apt-helper|g")
[ -n "$hook_command" ]
sh -c "$hook_command" >"$tmpdir/hook-output" 2>"$tmpdir/hook-error"
[ ! -s "$tmpdir/hook-output" ]
[ ! -s "$tmpdir/hook-error" ]

printf '#!/bin/sh\nprintf "synchronized\\n"\n' > "$tmpdir/apt-helper"
chmod 644 "$tmpdir/apt-helper"
sh -c "$hook_command" >"$tmpdir/hook-output" 2>"$tmpdir/hook-error"
[ ! -s "$tmpdir/hook-output" ]
[ ! -s "$tmpdir/hook-error" ]

chmod 755 "$tmpdir/apt-helper"
sh -c "$hook_command" >"$tmpdir/hook-output" 2>"$tmpdir/hook-error"
[ "$(cat "$tmpdir/hook-output")" = 'synchronized' ]
[ ! -s "$tmpdir/hook-error" ]

printf '#!/bin/sh\nexit 1\n' > "$tmpdir/apt-helper"
sh -c "$hook_command" >"$tmpdir/hook-output" 2>"$tmpdir/hook-error"
[ ! -s "$tmpdir/hook-output" ]
grep -q 'unable to synchronize Apport native origins; continuing apt update' "$tmpdir/hook-error"

missing_native_origins_dir="$tmpdir/missing-native-origins.d"
missing_origin_files_dir="$tmpdir/missing-origin-files"
mkdir -p "$missing_native_origins_dir" "$missing_origin_files_dir"
"$helper" "$sources_list" "$sources_list_dir" "$missing_native_origins_dir" "$missing_origin_files_dir" 2>"$tmpdir/missing-origin-error"
grep -q 'Apport native-origin file is missing' "$tmpdir/missing-origin-error"
for ppa in authd authd-candidate authd-edge authd-dev; do
    [ ! -e "$missing_native_origins_dir/$ppa" ]
    [ ! -L "$missing_native_origins_dir/$ppa" ]
done

# Run the source maintainer scripts without touching system configuration.
mkdir -p "$tmpdir/bin" "$tmpdir/usr/lib/authd"
printf '#!/bin/sh\nexit 0\n' > "$tmpdir/bin/pam-auth-update"
chmod 755 "$tmpdir/bin/pam-auth-update"
cp "$tmpdir/bin/pam-auth-update" "$tmpdir/usr/lib/authd/rename-authd-ppa-source.sh"
cp "$tmpdir/apt-helper" "$tmpdir/usr/lib/authd/sync-authd-apport-native-origin.sh"
sed -e "s|/etc/|$tmpdir/etc/|g" \
    -e "s|/usr/|$tmpdir/usr/|g" \
    -e "s|/var/|$tmpdir/var/|g" \
    "$postinst" > "$tmpdir/postinst"
PATH="$tmpdir/bin:$PATH" sh "$tmpdir/postinst" configure 999 \
    >"$tmpdir/postinst-output" 2>"$tmpdir/postinst-error"
grep -q 'W: could not update authd bug-reporting settings' "$tmpdir/postinst-error"

sed -e "s|/etc/|$tmpdir/etc/|g" \
    -e "s|/usr/|$tmpdir/usr/|g" \
    -e "s|/var/|$tmpdir/var/|g" \
    "$postrm" > "$tmpdir/postrm"
native_origins_dir="$tmpdir/etc/apport/native-origins.d"
mkdir -p "$native_origins_dir"

for action in remove purge disappear; do
    for origin_files_state in installed removed; do
        for ppa in authd authd-candidate authd-edge authd-dev; do
            native_origin_file="$tmpdir/usr/lib/authd/$ppa-native-origins"
            if [ "$origin_files_state" = installed ]; then
                cp "$origin_files_dir/$ppa-native-origins" "$native_origin_file"
            else
                rm "$native_origin_file"
            fi
        done
        for entry_type in symlink regular; do
            for ppa in authd authd-candidate authd-edge authd-dev; do
                if [ "$entry_type" = symlink ]; then
                    ln -s "$tmpdir/usr/lib/authd/$ppa-native-origins" "$native_origins_dir/$ppa"
                else
                    cp "$origin_files_dir/$ppa-native-origins" "$native_origins_dir/$ppa"
                fi
            done
            DPKG_MAINTSCRIPT_PACKAGE_REFCOUNT=1 sh "$tmpdir/postrm" "$action" \
                >"$tmpdir/postrm-output" 2>"$tmpdir/postrm-error"
            assert_only_enabled
            DPKG_MAINTSCRIPT_PACKAGE_REFCOUNT=1 sh "$tmpdir/postrm" "$action" \
                >"$tmpdir/postrm-output" 2>"$tmpdir/postrm-error"
            assert_only_enabled
        done
    done

    printf 'custom origin\n' > "$native_origins_dir/authd"
    cp "$origin_files_dir/authd-candidate-native-origins" "$tmpdir/admin-origin"
    ln -s "$tmpdir/admin-origin" "$native_origins_dir/authd-candidate"
    ln -s "$tmpdir/missing-admin-origin" "$native_origins_dir/authd-edge"
    printf 'custom dev origin\n' > "$native_origins_dir/authd-dev"
    DPKG_MAINTSCRIPT_PACKAGE_REFCOUNT=1 sh "$tmpdir/postrm" "$action" \
        >"$tmpdir/postrm-output" 2>"$tmpdir/postrm-error"
    [ ! -L "$native_origins_dir/authd" ]
    [ "$(cat "$native_origins_dir/authd")" = 'custom origin' ]
    [ "$(readlink "$native_origins_dir/authd-candidate")" = "$tmpdir/admin-origin" ]
    cmp "$tmpdir/admin-origin" "$origin_files_dir/authd-candidate-native-origins"
    [ "$(readlink "$native_origins_dir/authd-edge")" = "$tmpdir/missing-admin-origin" ]
    [ ! -L "$native_origins_dir/authd-dev" ]
    [ "$(cat "$native_origins_dir/authd-dev")" = 'custom dev origin' ]
    for ppa in authd authd-candidate authd-edge authd-dev; do
        rm "$native_origins_dir/$ppa"
    done
done

for ppa in authd authd-candidate authd-edge authd-dev; do
    ln -s "$tmpdir/usr/lib/authd/$ppa-native-origins" "$native_origins_dir/$ppa"
done
for action in upgrade failed-upgrade abort-install abort-upgrade remove purge; do
    refcount=1
    case "$action" in
        remove|purge) refcount=2 ;;
    esac
    DPKG_MAINTSCRIPT_PACKAGE_REFCOUNT="$refcount" sh "$tmpdir/postrm" "$action" \
        >"$tmpdir/postrm-output" 2>"$tmpdir/postrm-error"
    for ppa in authd authd-candidate authd-edge authd-dev; do
        [ "$(readlink "$native_origins_dir/$ppa")" = "$tmpdir/usr/lib/authd/$ppa-native-origins" ]
    done
done
