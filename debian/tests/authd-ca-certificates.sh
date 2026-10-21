#!/bin/sh
set -eu

hook=${1:-/etc/ca-certificates/update.d/authd}
if [ ! -d /run/systemd/system ]; then
	echo "E: this test requires a container or VM running systemd" >&2
	exit 1
fi

tmpdir=$(mktemp -d)
checksum_file=/var/lib/authd/ca-certificates.sha256
certificate=/usr/local/share/ca-certificates/authd-ca-test.crt
broker=snap.authd-ca-test.authd-ca-test.service
other_app=snap.authd-ca-test.broker.service
inactive=snap.authd-ca-test.inactive.service
unrelated=authd-ca-test.service

cleanup() {
	set +e
	systemctl stop "$broker" "$other_app" "$inactive" "$unrelated"
	for unit in "$broker" "$other_app" "$inactive" "$unrelated"; do
		rm -f "/run/systemd/system/$unit"
	done
	systemctl daemon-reload
	rm -f "$certificate"
	update-ca-certificates --hooksdir "$tmpdir/empty-hooks"
	rm -rf "$tmpdir"
}
trap cleanup 0

mkdir "$tmpdir/hooks" "$tmpdir/empty-hooks"
ln -s "$(realpath "$hook")" "$tmpdir/hooks/authd"
for unit in "$broker" "$other_app" "$inactive" "$unrelated"; do
	cat >"/run/systemd/system/$unit" <<EOF
[Unit]
Description=authd CA update test
StartLimitIntervalSec=0
[Service]
Type=oneshot
RemainAfterExit=yes
ExecStart=/usr/bin/test ! -e $tmpdir/fail-$unit
EOF
done
systemctl daemon-reload
systemctl start "$broker" "$other_app" "$unrelated"

invocation() {
	systemctl show --property=InvocationID --value "$1"
}

check_inactive() {
	if systemctl is-active --quiet "$1"; then
		echo "E: $1 should not be active" >&2
		exit 1
	fi
}

check_untouched() {
	[ "$(invocation "$unrelated")" = "$unrelated_id" ]
	check_inactive "$inactive"
}

check_restarted() {
	systemctl is-active --quiet "$broker"
	systemctl is-active --quiet "$other_app"
	[ "$(invocation "$broker")" != "$broker_id" ]
	[ "$(invocation "$other_app")" != "$other_app_id" ]
	check_untouched
}

new_certificate() {
	if ! openssl req -x509 -newkey rsa:2048 -nodes -config /dev/null \
		-keyout "$tmpdir/key" -out "$certificate" -days 1 \
		-addext basicConstraints=critical,CA:TRUE \
		-subj "/CN=authd CA test $1" >"$tmpdir/openssl.log" 2>&1; then
		cat "$tmpdir/openssl.log" >&2
		exit 1
	fi
}

update-ca-certificates --hooksdir "$tmpdir/empty-hooks"
echo "A missing checksum reloads active brokers before recording the bundle"
rm -f "$checksum_file"
broker_id=$(invocation "$broker")
other_app_id=$(invocation "$other_app")
unrelated_id=$(invocation "$unrelated")
"$hook" </dev/null
check_restarted
[ "$(cat "$checksum_file")" = "$(sha256sum /etc/ssl/certs/ca-certificates.crt)" ]
broker_id=$(invocation "$broker")
other_app_id=$(invocation "$other_app")

echo "Malformed and partial checksums reload active brokers"
for checksum in malformed "$(sha256sum /etc/ssl/certs/ca-certificates.crt | cut -c1-32)"; do
	printf '%s\n' "$checksum" >"$checksum_file"
	"$hook" </dev/null
	check_restarted
	[ "$(cat "$checksum_file")" = "$(sha256sum /etc/ssl/certs/ca-certificates.crt)" ]
	broker_id=$(invocation "$broker")
	other_app_id=$(invocation "$other_app")
done

echo "Unchanged bundles do not restart brokers"
update-ca-certificates --hooksdir "$tmpdir/hooks"
[ "$(invocation "$broker")" = "$broker_id" ]
[ "$(invocation "$other_app")" = "$other_app_id" ]
check_untouched

echo "A removed package leaves the hook a no-op"
unshare --mount --propagation private sh -eu -s -- "$hook" <<'EOF'
mount --bind /dev/null /usr/bin/authctl
"$1" </dev/null
EOF
[ ! -e "$checksum_file" ]
[ "$(invocation "$broker")" = "$broker_id" ]
[ "$(invocation "$other_app")" = "$other_app_id" ]
check_untouched
"$hook" </dev/null
check_restarted
broker_id=$(invocation "$broker")
other_app_id=$(invocation "$other_app")

echo "A missing CA bundle leaves the hook a no-op"
unshare --mount --propagation private sh -eu -s -- "$hook" "$tmpdir/empty-hooks" <<'EOF'
mount --bind "$2" /etc/ssl/certs
"$1" </dev/null
EOF
[ ! -e "$checksum_file" ]
[ "$(invocation "$broker")" = "$broker_id" ]
[ "$(invocation "$other_app")" = "$other_app_id" ]
check_untouched
"$hook" </dev/null
check_restarted
broker_id=$(invocation "$broker")
other_app_id=$(invocation "$other_app")

echo "Added certificates restart active brokers, including different app names"
new_certificate added
update-ca-certificates --hooksdir "$tmpdir/hooks"
check_restarted
broker_id=$(invocation "$broker")
other_app_id=$(invocation "$other_app")

echo "Replacing a certificate in place restarts brokers despite an empty change list"
new_certificate replaced
update-ca-certificates --hooksdir "$tmpdir/hooks" >"$tmpdir/update.log" 2>&1
grep -q '^0 added, 0 removed; done.$' "$tmpdir/update.log"
check_restarted

echo "Failed restarts leave the checksum unchanged and do not stop other restarts"
previous_checksum=$(cat "$checksum_file")
other_app_id=$(invocation "$other_app")
touch "$tmpdir/fail-$broker"
rm "$certificate"
update-ca-certificates --hooksdir "$tmpdir/empty-hooks"
if "$hook" </dev/null >"$tmpdir/error" 2>&1; then
	echo "E: the hook succeeded despite a failed broker restart" >&2
	exit 1
fi
grep -q "E: could not restart authd broker service $broker" "$tmpdir/error"
[ "$(cat "$checksum_file")" = "$previous_checksum" ]
[ "$(invocation "$other_app")" != "$other_app_id" ]
check_untouched

echo "A later invocation retries the changed bundle"
rm "$tmpdir/fail-$broker"
systemctl reset-failed "$broker"
systemctl start "$broker"
broker_id=$(invocation "$broker")
"$hook" </dev/null
[ "$(invocation "$broker")" != "$broker_id" ]
[ "$(cat "$checksum_file")" = "$(sha256sum /etc/ssl/certs/ca-certificates.crt)" ]

echo "An empty list of active brokers still records the updated bundle"
systemctl stop "$broker" "$other_app"
new_certificate no-brokers
update-ca-certificates --hooksdir "$tmpdir/hooks"
[ "$(cat "$checksum_file")" = "$(sha256sum /etc/ssl/certs/ca-certificates.crt)" ]
check_inactive "$broker"
check_inactive "$other_app"
check_untouched
