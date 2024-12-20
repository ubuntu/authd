# Configure authd

## Broker discovery

Create the directory that will contain the declaration files of the broker and copy the one from the Entra ID broker snap package:

```shell
sudo mkdir -p /etc/authd/brokers.d/
sudo cp /snap/authd-msentraid/current/conf/authd/msentraid.conf /etc/authd/brokers.d/
```

This file is used to declare the brokers available on the system. Several brokers can be enabled at the same time.

## Entra ID configuration

Register a new application in the Microsoft Azure portal. Once the application is registered, note the `Application (client) ID` and the `Directory (tenant) ID` from the `Overview` menu. These IDs are respectively the `<CLIENT_ID>` and `<ISSUER_ID>` used in the broker configuration file described in this document.

To register a new application, in Entra, select the menu `Identity > Applications > App registration`

![Menu showing selection of App registrations under Applications.](../assets/app-registration.png)

Then `New registration`

![User interface showing selection of New registration in App registrations.](../assets/new-registration.png)

And configure it as follows:

![Configuration screen for the new registration.](../assets/configure-registration.png)

Under `Manage`, in the `API permissions` menu, set the following Microsoft Graph permissions:

![Configuration screen for Microsoft Graph permissions.](../assets/graph-permissions.png)

Ensure the API permission type is set to **Delegated** for each permission.

Finally, as the supported authentication mechanism is the device workflow, you need to allow the public client workflows. Under `Manage`, in the `Authentication` menu, under `Advanced settings`, ensure that `Allow public client flows` is set to **Yes**.

[The Microsoft documentation](https://learn.microsoft.com/en-us/entra/identity-platform/quickstart-register-app) provides detailed instructions for registering an application with the Microsoft identity platform.

## Broker configuration

To configure the broker, edit the file `/var/snap/authd-msentraid/current/broker.conf`:

```ini
[oidc]
issuer = https://login.microsoftonline.com/<ISSUER_ID>/v2.0
client_id = <CLIENT_ID>

[users]
## The directory where the home directory will be created for new users.
## Existing users will keep their current directory.
## The user home directory will be created in the format of {home_base_dir}/{username}
#home_base_dir = /home

## The username suffixes that are allowed to login via ssh without existing previously in the system.
## The suffixes must be separated by commas.
#ssh_allowed_suffixes = @example.com,@anotherexample.com

## 'allowed_users' specifies the users who are permitted to log in after
## successfully authenticating with the Identity Provider.
## Values are separated by commas. Supported values:
## - 'OWNER': Grants access to the user specified in the 'owner' option
##        	(see below). This is the default.
## - 'ALL': Grants access to all users who successfully authenticate
##      	with the Identity Provider.
## - <username>: Grants access to specific additional users
##           	(e.g. user1@example.com).
## Example: allowed_users = OWNER,user1@example.com,admin@example.com
#allowed_users = OWNER

## 'owner' specifies the user assigned the owner role. This user is
## permitted to log in if 'OWNER' is included in the 'allowed_users'
## option.
##
## If this option is left unset, the first user to successfully log in
## via this broker will automatically be assigned the owner role. A
## drop-in configuration file will be created in broker.conf.d/ to set
## the 'owner' option.
##
## To disable automatic assignment, you can either:
## 1. Explicitly set this option to an empty value (e.g. owner = "")
## 2. Remove 'OWNER' from the 'allowed_users' option
##
## Example: owner = user2@example.com
#owner =
```

The broker configuration consists of two sections: the `oidc` section and the `users` section.

### OIDC section

The `oidc` section configures how the authd-broker will interact with the OIDC Provider.

Replace `<ISSUER_ID>` and `<CLIENT_ID>` by the issuer ID and client ID retrieved from the MS Entra ID portal.

### Users section

The `users` section configures which users will be allowed to access the machine.

By default the first person to log in to the machine is automatically registered
as the owner. If you wish to override this behaviour then specify a list of allowed
users with the `allowed_users` keyword, while omitting the `OWNER` keyword:

```text
allowed_users = person1@email.com,person2@email.com
```

Alternatively, you can directly register someone as the owner by using the `owner`
keyword:

```text
owner = your@email.com
```

Explicitly setting an empty owner, has the same effect as omitting the `OWNER` keyword
in `allowed_users`:

```text
owner =
```

### Apply your configuration changes

When a configuration file is added you have to restart authd:

```shell
sudo systemctl restart authd
```

When the configuration of a broker is updated, you have to restart the broker:

```shell
sudo snap restart authd-msentraid
```

## System configuration

By default on Ubuntu, the login timeout is 60s. This may be too brief for a device code flow authentication. It can be set to a different value by changing the value of `LOGIN_TIMEOUT` in `/etc/login.defs`
