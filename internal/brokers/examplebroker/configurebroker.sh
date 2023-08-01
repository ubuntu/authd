#!/bin/bash
# This script needs sudo privileges

set -eu

# Copy the configuration file to /etc/authd/broker.d/ExampleBroker
cp ./ExampleBroker /etc/authd/broker.d/ExampleBroker

# Copy the interface definition to /usr/share/dbus-1/interfaces
cp ./com.ubuntu.auth.ExampleBroker.xml /usr/share/dbus-1/interfaces/com.ubuntu.auth.ExampleBroker.xml

# Copy the dbus configuration file to /usr/share/dbus-1/system.d/
cp ./com.ubuntu.auth.ExampleBroker.conf /usr/share/dbus-1/system.d/com.ubuntu.auth.ExampleBroker.conf
