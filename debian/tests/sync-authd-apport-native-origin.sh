#!/bin/sh
set -eu

helper=${1:-/usr/lib/authd/sync-authd-apport-native-origin.sh}
origin_file=${2:-/usr/lib/authd/authd-native-origins}
apt_hook=${3:-/etc/apt/apt.conf.d/99authd-apport-native-origin}
postinst=${4:-debian/postinst}
tmpdir=$(mktemp -d)
trap 'rm -rf "$tmpdir"' 0

sources_list="$tmpdir/sources.list"
sources_list_dir="$tmpdir/sources.list.d"
native_origins_dir="$tmpdir/native-origins.d"
native_origin_entry="$native_origins_dir/authd"
mkdir -p "$sources_list_dir" "$native_origins_dir"

sync_origin() {
    "$helper" "$sources_list" "$sources_list_dir" "$native_origins_dir" "$origin_file"
}

assert_enabled() {
    [ -L "$native_origin_entry" ]
    [ "$(readlink "$native_origin_entry")" = "$origin_file" ]
}

assert_disabled() {
    [ ! -e "$native_origin_entry" ]
    [ ! -L "$native_origin_entry" ]
}

sync_origin
assert_disabled

cat > "$sources_list_dir/authd.sources" <<'EOF'
Types: deb
URIs: https://archive.ubuntu.com/ubuntu/
  https://ppa.launchpadcontent.net/ubuntu-enterprise-desktop/authd/ubuntu/
Suites: noble
Components: main
EOF
sync_origin
assert_enabled
sync_origin
assert_enabled
printf 'Types: deb\r\nURIs: https://ppa.launchpadcontent.net/ubuntu-enterprise-desktop/authd/ubuntu/\r\nSuites: noble\r\nComponents: main\r\n\r\nTypes: deb\r\nURIs: https://archive.ubuntu.com/ubuntu/\r\nSuites: noble\r\nComponents: main\r\nEnabled: no\r\n' > "$sources_list_dir/authd.sources"
sync_origin
assert_enabled

printf 'Types: deb\r\nURIs: https://archive.ubuntu.com/ubuntu/\r\nSuites: noble\r\nComponents: main\r\nEnabled: no\r\n\r\nTypes: deb\r\nURIs: https://ppa.launchpadcontent.net/ubuntu-enterprise-desktop/authd/ubuntu/\r\nSuites: noble\r\nComponents: main\r\n' > "$sources_list_dir/authd.sources"
sync_origin
assert_enabled

cat > "$sources_list_dir/authd.sources" <<'EOF'
Types: deb
URIs: https://ppa.launchpadcontent.net/ubuntu-enterprise-desktop/authd/ubuntu/
Suites: noble
Components: main
Enabled: no
EOF
sync_origin
assert_disabled

sed '/^Enabled:/d' "$sources_list_dir/authd.sources" > "$tmpdir/disabled.sources"
for disabled in no false off 0 NO False; do
    for separator in ' ' '\n '; do
        cp "$tmpdir/disabled.sources" "$sources_list_dir/authd.sources"
        printf 'Enabled:%b%s\n' "$separator" "$disabled" >> "$sources_list_dir/authd.sources"
        sync_origin
        assert_disabled
    done
done

cp "$tmpdir/disabled.sources" "$sources_list_dir/authd.sources"
sync_origin
assert_enabled
mv "$sources_list_dir/authd.sources" "$sources_list_dir/authd disabled.sources"
sync_origin
assert_disabled
mv "$sources_list_dir/authd disabled.sources" "$sources_list_dir/authd.sources"

cat > "$sources_list_dir/authd.sources" <<'EOF'
Types: deb
URIs: https://ppa.launchpadcontent.net/ubuntu-enterprise-desktop/authd-edge/ubuntu/
Suites: noble
Components: main
EOF
sync_origin
assert_disabled

rm "$sources_list_dir/authd.sources"
cat > "$sources_list_dir/authd.list" <<'EOF'
deb [arch=amd64] http://ppa.launchpad.net/ubuntu-enterprise-desktop/authd/ubuntu/ noble main
EOF
sync_origin
assert_enabled

mv "$sources_list_dir/authd.list" "$sources_list_dir/authd@disabled.list"
sync_origin
assert_disabled
mv "$sources_list_dir/authd@disabled.list" "$sources_list_dir/authd.list"

rm "$sources_list_dir/authd.list"
cat > "$sources_list" <<'EOF'
deb https://ppa.launchpadcontent.net/ubuntu-enterprise-desktop/authd/ubuntu noble main
EOF
sync_origin
assert_enabled

: > "$sources_list"
sync_origin
assert_disabled

cp "$origin_file" "$native_origin_entry"
sync_origin
assert_disabled

cp "$origin_file" "$tmpdir/admin-origin"
ln -s "$tmpdir/admin-origin" "$native_origin_entry"
sync_origin
[ -L "$native_origin_entry" ]
[ "$(readlink "$native_origin_entry")" = "$tmpdir/admin-origin" ]
rm "$tmpdir/admin-origin"
sync_origin
[ -L "$native_origin_entry" ]
[ "$(readlink "$native_origin_entry")" = "$tmpdir/admin-origin" ]
rm "$native_origin_entry"

cat > "$sources_list_dir/authd.sources" <<'EOF'
Types: deb
URIs: https://ppa.launchpadcontent.net/ubuntu-enterprise-desktop/authd/ubuntu/
Suites: noble
Components: main
EOF
printf 'custom origin\n' > "$native_origin_entry"
sync_origin 2>"$tmpdir/error"
[ "$(cat "$native_origin_entry")" = 'custom origin' ]
grep -q 'not replacing existing Apport native-origin entry' "$tmpdir/error"

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

# Run the source maintainer script without touching system configuration.
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
