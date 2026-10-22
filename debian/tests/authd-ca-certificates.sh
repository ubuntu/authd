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
	systemctl disable --runtime "$broker"
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
[Install]
WantedBy=multi-user.target
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

echo "Restoring the previous bundle after an offline update restarts brokers"
previous_checksum=$(cat "$checksum_file")
new_certificate offline
update-ca-certificates --hooksdir "$tmpdir/empty-hooks"
[ "$(sha256sum /etc/ssl/certs/ca-certificates.crt)" != "$previous_checksum" ]
unshare --mount --propagation private sh -eu -s -- "$hook" <<'EOF'
mount -t tmpfs tmpfs /run
"$1" </dev/null
EOF
[ ! -e "$checksum_file" ]
[ "$(invocation "$broker")" = "$broker_id" ]
[ "$(invocation "$other_app")" = "$other_app_id" ]
check_untouched
systemctl restart "$broker" "$other_app"
broker_id=$(invocation "$broker")
other_app_id=$(invocation "$other_app")
rm "$certificate"
update-ca-certificates --hooksdir "$tmpdir/hooks"
check_restarted
[ "$(cat "$checksum_file")" = "$previous_checksum" ]
broker_id=$(invocation "$broker")
other_app_id=$(invocation "$other_app")

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

echo "Failed service discovery invalidates the checksum before the bundle returns"
previous_checksum=$(cat "$checksum_file")
new_certificate discovery-failure
update-ca-certificates --hooksdir "$tmpdir/empty-hooks"
[ "$(sha256sum /etc/ssl/certs/ca-certificates.crt)" != "$previous_checksum" ]
mkdir "$tmpdir/discovery-bin"
cat >"$tmpdir/discovery-bin/systemctl" <<'EOF'
#!/bin/sh
exit 1
EOF
chmod +x "$tmpdir/discovery-bin/systemctl"
if PATH="$tmpdir/discovery-bin:$PATH" "$hook" </dev/null >"$tmpdir/error" 2>&1; then
	echo "E: the hook succeeded despite failed service discovery" >&2
	exit 1
fi
grep -q "E: could not list active authd broker services" "$tmpdir/error"
[ ! -e "$checksum_file" ]
[ "$(invocation "$broker")" = "$broker_id" ]
[ "$(invocation "$other_app")" = "$other_app_id" ]
check_untouched
systemctl restart "$broker" "$other_app"
broker_id=$(invocation "$broker")
other_app_id=$(invocation "$other_app")
rm "$certificate"
update-ca-certificates --hooksdir "$tmpdir/hooks"
check_restarted
[ "$(cat "$checksum_file")" = "$previous_checksum" ]
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

echo "Failed restarts invalidate the checksum and do not stop other restarts"
other_app_id=$(invocation "$other_app")
touch "$tmpdir/fail-$broker"
rm "$certificate"
update-ca-certificates --hooksdir "$tmpdir/empty-hooks"
if "$hook" </dev/null >"$tmpdir/error" 2>&1; then
	echo "E: the hook succeeded despite a failed broker restart" >&2
	exit 1
fi
grep -q "E: could not restart authd broker service $broker" "$tmpdir/error"
[ ! -e "$checksum_file" ]
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

echo "Restoring the previous bundle after a partial failure still restarts brokers"
previous_checksum=$(cat "$checksum_file")
other_app_id=$(invocation "$other_app")
new_certificate partial-failure
touch "$tmpdir/fail-$broker"
update-ca-certificates --hooksdir "$tmpdir/empty-hooks"
[ "$(sha256sum /etc/ssl/certs/ca-certificates.crt)" != "$previous_checksum" ]
if "$hook" </dev/null >"$tmpdir/error" 2>&1; then
	echo "E: the hook succeeded despite a failed broker restart" >&2
	exit 1
fi
grep -q "E: could not restart authd broker service $broker" "$tmpdir/error"
[ ! -e "$checksum_file" ]
[ "$(invocation "$other_app")" != "$other_app_id" ]
check_untouched
rm "$certificate" "$tmpdir/fail-$broker"
update-ca-certificates --hooksdir "$tmpdir/empty-hooks"
[ "$(sha256sum /etc/ssl/certs/ca-certificates.crt)" = "$previous_checksum" ]
systemctl reset-failed "$broker"
systemctl start "$broker"
broker_id=$(invocation "$broker")
other_app_id=$(invocation "$other_app")
"$hook" </dev/null
check_restarted
[ "$(cat "$checksum_file")" = "$previous_checksum" ]

echo "Policy-denied restarts invalidate the checksum so they can be retried"
mkdir -p "$tmpdir/policy/usr/sbin"
cat >"$tmpdir/policy/usr/sbin/policy-rc.d" <<'EOF'
#!/bin/sh
exit 101
EOF
chmod +x "$tmpdir/policy/usr/sbin/policy-rc.d"
broker_id=$(invocation "$broker")
other_app_id=$(invocation "$other_app")
new_certificate policy
update-ca-certificates --hooksdir "$tmpdir/empty-hooks"
if DPKG_ROOT="$tmpdir/policy" "$hook" </dev/null >"$tmpdir/error" 2>&1; then
	echo "E: the hook succeeded despite policy-denied broker restarts" >&2
	exit 1
fi
grep -q "E: authd broker service $broker is still running" "$tmpdir/error"
[ ! -e "$checksum_file" ]
[ "$(invocation "$broker")" = "$broker_id" ]
[ "$(invocation "$other_app")" = "$other_app_id" ]
check_untouched
"$hook" </dev/null
check_restarted
[ "$(cat "$checksum_file")" = "$(sha256sum /etc/ssl/certs/ca-certificates.crt)" ]

echo "A broker stopped after discovery is not started again, even when enabled"
mkdir "$tmpdir/bin"
real_invoke=$(command -v deb-systemd-invoke)
cat >"$tmpdir/bin/deb-systemd-invoke" <<EOF
#!/bin/sh
set -eu
if [ "\$2" = "$broker" ]; then
	systemctl stop "\$2"
fi
exec "$real_invoke" "\$@"
EOF
chmod +x "$tmpdir/bin/deb-systemd-invoke"
systemctl enable --runtime "$broker"
other_app_id=$(invocation "$other_app")
new_certificate stopped
update-ca-certificates --hooksdir "$tmpdir/empty-hooks"
PATH="$tmpdir/bin:$PATH" "$hook" </dev/null
check_inactive "$broker"
[ "$(invocation "$other_app")" != "$other_app_id" ]
[ "$(cat "$checksum_file")" = "$(sha256sum /etc/ssl/certs/ca-certificates.crt)" ]
check_untouched

echo "An empty list of active brokers still records the updated bundle"
systemctl stop "$broker" "$other_app"
new_certificate no-brokers
update-ca-certificates --hooksdir "$tmpdir/hooks"
[ "$(cat "$checksum_file")" = "$(sha256sum /etc/ssl/certs/ca-certificates.crt)" ]
check_inactive "$broker"
check_inactive "$other_app"
check_untouched
