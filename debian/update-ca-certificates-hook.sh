#!/bin/sh
set -eu

# Broker snaps use the host CA store on Ubuntu and cache TLS roots in-process.
checksum_file=/var/lib/authd/ca-certificates.sha256
if [ ! -d /run/systemd/system ] || [ ! -x /usr/bin/authctl ] || [ ! -f /etc/ssl/certs/ca-certificates.crt ]; then
	rm -f "$checksum_file"
	exit 0
fi

if sha256sum --status -c "$checksum_file" 2>/dev/null; then
	exit 0
fi
checksum=$(sha256sum /etc/ssl/certs/ca-certificates.crt)

active_units=$(systemctl show 'snap.authd-*.service' \
	--state=active \
	--property=Id \
	--value \
	--no-pager) || {
	echo "E: could not list active authd broker services" >&2
	exit 1
}

status=0
for unit in $active_units; do
	echo "Restarting $unit to reload the updated CA certificates."
	if ! deb-systemd-invoke restart "$unit"; then
		echo "E: could not restart authd broker service $unit" >&2
		status=1
	fi
done

if [ "$status" -ne 0 ]; then
	exit "$status"
fi

# Only record the bundle once every active broker has reloaded it.
if [ ! -d /var/lib/authd ]; then
	mkdir -m 0700 /var/lib/authd
fi
printf '%s\n' "$checksum" >"$checksum_file"
