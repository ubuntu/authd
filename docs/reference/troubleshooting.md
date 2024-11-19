# Troubleshooting

## Logging

```authd``` and the brokers are all logging to the system journal.

For ```authd``` entries, run:

```shell
journalctl -u authd.service
```

For the MS Entra ID broker entries, run:

```shell
journalctl -u snap.authd-msentraid.authd-msentraid.service
```

For the GDM integration:

```shell
journalctl /usr/bin/gnome-shell
```

For anything else or more broader investigation, use ```journalctl```.

### Logging verbosity

You can increase the verbosity of the logs in different ways.

#### PAM module

Append ```debug=true``` to all the lines with `pam_authd_exec.so` or `pam_authd.so` in the PAM configuration files (`common-auth`, `gdm-authd`...) in ```/etc/pam.d/``` to increase the verbosity of the PAM messages.

#### NSS module

Export `AUTHD_NSS_INFO=stderr` environment variable on any program using the authd NSS module to get more info on NSS requests to authd.

#### authd service

To increase the verbosity of the service itself, edit the service file:

```shell
sudo systemctl edit authd.service
```

Add the following lines to the override file and make sure to add `-vv` at the end of the `authd` command:

```
[Service]
ExecStart=
ExecStart=/usr/libexec/authd -vv
```

Then you need to restart the service with `sudo systemctl restart authd`.

#### GDM

Ensure the lines in `/etc/gdm3/custom.conf` are not commented:

```ini
[debug]
# Uncomment the line below to turn on debugging
# More verbose logs
# Additionally lets the X server dump core if it crashes
Enable=true
```

Then you need to restart the service with `sudo systemctl restart gdm`.

#### authd-msentraid service

To increase the verbosity of the broker service, edit the service file:

```shell
sudo systemctl edit snap.authd-msentraid.authd-msentraid.service
```

Add the following lines to the override file and make sure to add `-vv` to the exec command:

```
[Service]
ExecStart=
ExecStart=/usr/bin/snap run authd-msentraid -vv
```

You will then need to restart the service with `snap restart authd-msentraid`.

## Switch the snap to the edge channel

Maybe your issue is already fixed! You should try switching to the edge channel of the broker snap. You can easily do that with:

```shell
snap switch authd-msentraid --edge
snap refresh authd-msentraid
```

Keep in mind that this version is not tested and may be incompatible with current released version of authd. You should switch back to stable after trying the edge channel:

```shell
snap switch authd-msentraid --stable
snap refresh authd-msentraid
```

## Common issues and limitations

### File ownership on shared network resources (e.g. NFS)

The user and group IDs assigned by authd are currently not guaranteed to be the same on different systems, so the same user can be assigned a different UID on different systems. That means that shared network resources like NFS which rely on UIDs and GIDs for access are currently not supported, because users might not be able to access their own files and might even be able to access files they should not be able to access.

We are looking into how we can support this in the future.
