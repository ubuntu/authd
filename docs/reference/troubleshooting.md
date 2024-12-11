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

### File ownership on shared network resources (e.g. NFS, Samba)

The user identifiers (UIDs) and group identifiers (GIDs) assigned by authd are
unique to each machine. This means that when using authd with NFS or Samba, the
UIDs and GIDs of users and groups on the server will not match those on the
client machines, which leads to permission issues.

To avoid these issues, you can use ID mapping. For more information, see
* [Using authd with NFS](../howto/use-with-nfs)
* [Using authd with Samba](../howto/use-with-samba)
