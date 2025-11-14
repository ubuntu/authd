# Log in with SSH

## Server configuration

To enable SSH access with `authd` you must configure `sshd` and the broker.

(ref::ssh-configuration)=
### SSH configuration

To configure SSH, create a file `/etc/ssh/sshd_config.d/authd.conf` with the following content:

```
UsePAM yes
KbdInteractiveAuthentication yes
```

Alternatively, you can directly set the keys in the sshd configuration file `/etc/ssh/sshd_config`.

Then restart the SSH server:

```shell
sudo systemctl restart ssh
```

### Broker configuration

By default, SSH only allows logins from users that already exist on the system.
New authd users (who have never logged in before) are *not* allowed to log in
for the first time via SSH unless this option is configured.

If configured, only users with a suffix in this list are allowed to
authenticate for the first time directly through SSH.
Note that this does not affect users that already authenticated for
the first time and already exist on the system.

To configure the broker edit the file `/var/snap/authd-<broker_name>/current/broker.conf` and set the key `ssh_allowed_suffixes_first_auth` with the list of domains that you want to allow.

```ini
...

[users]
## Suffixes must be comma-separated (e.g., '@example.com,@example.org').
## To allow all suffixes, use a single asterisk ('*').
`ssh_allowed_suffixes_first_auth` = <ALLOWED DOMAINS>
```

You can set several domains separated by a comma. For instance:

```ini
ssh_allowed_suffixes_first_auth = @example.com,@ubuntu.com
```

## Usage

Once this is all set up, you can ssh to the server in the same way that you would do with any server: `ssh <username>@<host>`. The format of `<username>` is the user handle on the provider, such as `user@domain.tld`.

For instance, here is an example using MS Entra ID as a provider:

```shell
ssh user@domain.tld@remote.host
```

![Terminal interface showing option to authentice by login code or QR scan when user tries to ssh into server](../assets/ssh-qr.png)
