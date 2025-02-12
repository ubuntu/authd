---
myst:
  html_meta:
    "description lang=en": "Install the authd authentication service and its identity brokers to enable Ubuntu devices to authenticate with multiple cloud identity providers, including Google IAM and Microsoft Entra ID."
---

(howto::install)=
# Install authd and brokers for cloud identity providers

This project consists of two components:
* **authd**: The authentication daemon responsible for managing access to the authentication mechanism.
* **identity broker**: The services that handle the interface with an identity provider. There can be several identity brokers installed and enabled on the system.

authd is delivered as a Debian package for Ubuntu Desktop and Ubuntu Server.

## System requirements

* Distribution: Ubuntu Desktop 24.04 LTS or Ubuntu Server 24.04 LTS
* Architectures: amd64, arm64

## Install authd

You can install authd from the [stable PPA](https://launchpad.net/~ubuntu-enterprise-desktop/+archive/ubuntu/authd).

To add this PPA to your system's software sources, run the following commands:

```shell
sudo add-apt-repository ppa:ubuntu-enterprise-desktop/authd
sudo apt update
```

```{note}
The stable PPA release of authd can be used today as an authentication service.
This project is under active development and the release of authd to the
official archive is planned for Ubuntu 26.04 LTS.
```

Then install authd and any additional Debian packages needed for your system of
choice:

:::::{tab-set}
:sync-group: system

::::{tab-item} Ubuntu Desktop
:sync: desktop

```shell
sudo apt install authd gnome-shell yaru-theme-gnome-shell
```
::::

::::{tab-item} Ubuntu Server
:sync: server

```shell
sudo apt install authd
```
::::
:::::

## Install brokers

The brokers are provided as Snap packages and are available from the Snap
Store.

### MS Entra ID broker

To install the MS Entra ID broker, run the following command:

```shell
sudo snap install authd-msentraid
```

At this stage, you have installed the main service and an identity broker to
authenticate against Microsoft Entra ID.

### Google IAM broker

To install the Google IAM broker, run the following command:

```shell
sudo snap install authd-google
```

At this stage, you have installed the main service and an identity broker to
authenticate against Google IAM.
