# Login with SSH

## Server configuration

To enable SSH access with `authd` you must configure `sshd` and the broker.

### SSH configuration

To configure SSH, create a file `/etc/ssh/sshd_config.d/authd.conf` with the following content:

```
UsePAM yes
KbdInteractiveAuthentication yes
```

Alternatively you can directly set the keys in the sshd configuration file `/etc/ssh/sshd_config`.

Then restart the SSH server:

```
sudo systemctl restart ssh
```

### Broker configuration

To configure the broker edit the file `/var/snap/authd-msentraid/current/broker.conf` and set the key `ssh_allowed_suffixes` with the list of domains that you want to allow.

```
[oidc]
issuer = https://login.microsoftonline.com/<ISSUER_ID>/v2.0
client_id = <CLIENT_ID>

[users]
# The directory where the home directory will be created for new users.
# Existing users will keep their current directory.
# The user home directory will be created in the format of {home_base_dir}/{username}
# home_base_dir = /home

# The username suffixes that are allowed to login via ssh without existing previously in the system.
# The suffixes must be separated by commas.
ssh_allowed_suffixes = <ALLOWED DOMAINS>
```

You can set several domains separated by a comma. For instance:

```
ssh_allowed_suffixes = @example.com,@ubuntu.com
```

## Usage

Once this is all set up, you can ssh to the server in the same way you'd do with any server: `ssh <username>@<host>`. The format of `<username>` is the user handle on Entra ID such as `user@domain.tld`.

For instance:

```shell
ssh user@domain.tld@remote.host
```

![Terminal interface showing option to authentice by login code or QR scan when user tries to ssh into server](../assets/ssh-qr.png)
