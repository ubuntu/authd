#!/bin/sh
set -eu

helper=${1:-/usr/lib/authd/rename-authd-ppa-source.sh}
tmpdir=$(mktemp -d)
old="$tmpdir/ubuntu-enterprise-desktop-ubuntu-authd-noble.sources"
new="$tmpdir/ubuntu-enterprise-desktop-ubuntu-authd.sources"
edge="$tmpdir/ubuntu-enterprise-desktop-ubuntu-authd-edge-noble.sources"
error="$tmpdir/error"
trap 'rm -rf "$tmpdir"' 0

cat > "$old" <<'EOF'
Types: deb
URIs: https://ppa.launchpadcontent.net/ubuntu-enterprise-desktop/authd/ubuntu/
Suites: resolute
Components: main
EOF

"$helper" "$tmpdir"
[ ! -e "$old" ]
[ -f "$new" ]
grep -q '^Suites: resolute$' "$new"

cat > "$old" <<'EOF'
Types: deb
URIs: https://ppa.launchpadcontent.net/ubuntu-enterprise-desktop/authd/ubuntu/
Suites: resolute
Components: main
EOF
printf 'keep target\n' > "$new"
"$helper" "$tmpdir" 2>"$error"
[ -f "$old" ]
[ "$(cat "$new")" = 'keep target' ]
grep -q 'target already exists' "$error"

rm -f "$old" "$new" "$error"
cat > "$edge" <<'EOF'
Types: deb
URIs: https://ppa.launchpadcontent.net/ubuntu-enterprise-desktop/authd-edge/ubuntu/
Suites: resolute
Components: main
EOF
"$helper" "$tmpdir"
[ -f "$edge" ]
[ ! -e "$new" ]
