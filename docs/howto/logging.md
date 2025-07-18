---
myst:
  html_meta:
    "description lang=en": "Get logs from authd and configure logging behavior."
---

(ref::logging)=
# Accessing and configuring logs

Logs are generated for authd and its brokers, which can help when
troubleshooting and reporting bugs.

## authd

`authd` logs to the system journal.

For `authd` entries, run:

```shell
sudo journalctl -u authd.service
```

If you want logs for authd and all brokers on the system, run:

```shell
sudo journalctl -u authd.service -u "snap.authd-*.service"
```

For specific broker entries run the command for your chosen broker:

::::{tab-set}
:sync-group: broker

:::{tab-item} Google IAM
:sync: google

```shell
sudo journalctl -u snap.authd-google.authd-google.service
```
:::

:::{tab-item} MS Entra ID
:sync: msentraid

```shell
sudo journalctl -u snap.authd-msentraid.authd-msentraid.service
```
:::
::::

For the GDM integration:

```shell
sudo journalctl /usr/bin/gnome-shell
```

For anything else or more broader investigation, use `sudo journalctl`.

## Configure logging verbosity

You can increase the verbosity of the logs in different ways.

### PAM module

Append `debug=true` to all the lines with `pam_authd_exec.so` or `pam_authd.so` in the PAM configuration files (`common-auth`, `gdm-authd`...) in `/etc/pam.d/` to increase the verbosity of the PAM messages.

### NSS module

Export `AUTHD_NSS_INFO=stderr` environment variable on any program using the authd NSS module to get more info on NSS requests to authd.

### authd service

To increase the verbosity of the service itself, edit the service file:

```shell
sudo systemctl edit authd.service
```

Add the following lines to the override file and make sure to add `-vv` at the end of the `authd` command:

```ini
[Service]
ExecStart=
ExecStart=/usr/libexec/authd -vv
```

Then you need to restart the service with `sudo systemctl restart authd`.

### GDM

Ensure the lines in `/etc/gdm3/custom.conf` are not commented:

```ini
[debug]
# Uncomment the line below to turn on debugging
# More verbose logs
# Additionally lets the X server dump core if it crashes
Enable=true
```

Then you need to restart the service with `sudo systemctl restart gdm`.

### authd broker service

To increase the verbosity of the broker service, edit the service file:

::::{tab-set}
:sync-group: broker

:::{tab-item} Google IAM
:sync: google

```shell
sudo systemctl edit snap.authd-google.authd-google.service
```
:::

:::{tab-item} MS Entra ID
:sync: msentraid

```shell
sudo systemctl edit snap.authd-msentraid.authd-msentraid.service
```
:::
::::

Add the following lines to the override file and make sure to add `-vv` to the exec command:

::::{tab-set}
:sync-group: broker

:::{tab-item} Google IAM
:sync: google

```ini
[Service]
ExecStart=
ExecStart=/usr/bin/snap run authd-google -vv
```
:::

:::{tab-item} MS Entra ID
:sync: msentraid

```ini
[Service]
ExecStart=
ExecStart=/usr/bin/snap run authd-msentraid -vv
```
::::

You will then need to restart the service with:

::::{tab-set}
:sync-group: broker

:::{tab-item} Google IAM
:sync: google

`sudo snap restart authd-google`.
:::

:::{tab-item} MS Entra ID
:sync: msentraid

`sudo snap restart authd-msentraid`.
:::
::::
