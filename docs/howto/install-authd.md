# Installation

This project consists of two components:
* authd: The authentication daemon responsible for managing access to the authentication mechanism.
* an identity broker: The services that handle the interface with an identity provider. There can be several identity brokers installed and enabled on the system.

authd is delivered as a Debian package.

## System requirements

* Distribution: Ubuntu Desktop 24.04 LTS or Ubuntu Server 24.04 LTS
* Architectures: amd64, arm64


```{note}
You can install authd from the [stable PPA](https://launchpad.net/~ubuntu-enterprise-desktop/+archive/ubuntu/authd).

You can add this PPA to your system's software sources with the following commands:

```shell
sudo add-apt-repository ppa:ubuntu-enterprise-desktop/authd
sudo apt update
```

Install the following Debian packages (note that `gnome-shell` and `yaru-theme*` are only required for desktop integration, server installations may ignore them):

```shell
sudo apt install authd gnome-shell yaru-theme-gnome-shell
```

## Install brokers

The brokers are provided as Snap packages and available from the Snap Store.

### MS Entra ID broker

To install the MS Entra ID broker, run the following command:

```shell
sudo snap install authd-msentraid
```

At this stage, you have installed the main service and an identity broker to authenticate against Microsoft Entra ID.

### Google IAM broker

To install the Google IAM broker, run the following command:

```shell
sudo snap install authd-google
```

At this stage, you have installed the main service and an identity broker to authenticate against Google IAM.
