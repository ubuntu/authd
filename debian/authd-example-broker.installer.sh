#!/bin/sh

set -eu

usage() {
    echo "$0 [install | uninstall | help]"
}

if [ -z "$0" ]; then
    usage
    exit 1
fi

if [ "$(id -u)" != 0 ]; then
    echo "Need to run as root"
    exit
fi

SYSTEMD_SERVICE=authd-example-broker.service
CONFIG_FILE=ExampleBroker.conf

if [ "$1" = "install" ]; then
    install -m644 \
        /usr/share/doc/authd-example-broker/examples/"${CONFIG_FILE}" \
        -Dt /etc/authd/brokers.d

    install -m644 \
        /usr/share/doc/authd-example-broker/examples/"${SYSTEMD_SERVICE}" \
        -Dt /usr/lib/systemd/system

    systemctl daemon-reload
elif [ "$1" = "uninstall" ]; then
    rm -fv /etc/authd/brokers.d/"${CONFIG_FILE}"
    rmdir -v /etc/authd/brokers.d 2>/dev/null || true
    rm -fv /usr/lib/systemd/system/"${SYSTEMD_SERVICE}"

    systemctl daemon-reload
elif [ "$1" = "help" ]; then
    usage
    exit 0
else
    echo "unknown command '$1'"
    usage
    exit 1
fi
