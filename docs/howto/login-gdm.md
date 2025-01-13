# Log in with GDM

## Logging in with a remote provider

Once the system is configured you can log into your system using your remote provider credentials and the device code flow.
In this example, we are going to use MS Entra ID as the remote provider but the process is equivalent for other providers.

> See all the available providers: [Install brokers](./install-authd.md#install-brokers)

In the login screen (greeter), select ```not listed``` below the user name field.

Type your remote provider user name. The format is ```user@domain.name```

Select the broker `Microsoft Entra ID`

![Login screen showing selection of broker.](../assets/gdm-select-broker.png)

If MFA is enabled, a QR code and a login code are displayed.

![Display of QR code, login code and button to Request new login code.](../assets/gdm-qr.png)

From a second device, flash the QR code or type the URL in a web browser, then follow your provider's authentication process.

Upon successful authentication, the user is prompted to enter a local password. This password can be used for offline authentication.

![Prompt to create local password on successful authentication.](../assets/gdm-pass.png)

## Group management

In our example the user `authd test` is a member of the Azure groups `Azure_OIDC_Test` and `linux-sudo`:

![Azure portal interface showing the Azure groups.](../assets/gdm-groups.png)

This translates to the following unix groups on the local machine:

```shell
~$ groups
aadtest-testauthd@uaadtest.onmicrosoft.com sudo azure_oidc_test
```

There are three types of groups:
1. **Primary group**: Created automatically based on the user name
1. **Local group**: Group local to the machine prefixed with `linux-`. For instance if the user is a member of the Azure group `linux-sudo`, they will be a member of the `sudo` group locally.
1. **Remote group**: All the other Azure groups the user is a member of.

## Commands

### authd

```authd``` is socket-activated. It means that the service starts on-demand when it receives a request on a socket.

If you want to restart the service, you can stop it with ```systemctl stop authd``` and it will restart automatically on the next message it receives.

Run ```/usr/libexec/authd --help``` to display the entire help.

## Broker management

The broker is managed through the ```snap``` command.

The main operation is to restart the broker to reload the configuration when it has changed. You can reload the broker with the command:

```shell
snap restart authd-msentraid
```

> If you are using a different broker to `msentraid`, make sure to change the snap name when running this command.
