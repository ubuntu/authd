#!/bin/sh

cat <<EOF | sudo tee /etc/dpkg/dpkg.cfg.d/01_nodoc
path-exclude=/usr/share/locale/*
path-exclude=/usr/share/man/*
path-exclude=/usr/share/doc/*
path-exclude=/usr/share/info/*
EOF
