# Login with GDM

## Logging in with a remote provider

Once the system is configured you can log into your system using your MS Entra ID credentials and the device code flow.

In the login screen (greeter), select ```not listed``` below the user name field.

Type your MS Entra ID user name. The format is ```user@domain.name```

Select the broker `Microsoft Entra ID`

![image](https://github.com/ubuntu/authd/assets/1928546/153f2d85-b5ed-4fb2-802c-ad0a10f0e598)

If MFA is enabled, a QR code and a login code are displayed.

![image](https://github.com/ubuntu/authd/assets/1928546/94daf869-2fcb-47e8-aeb9-2f71ff0c3ff9)

From a second device, flash the QR code or type the URL in a web browser, then follow the authentication process from your provider.

Upon successful authentication, the user is prompted to enter a local password. This password can be used for offline authentication.

![image](https://github.com/ubuntu/authd/assets/1928546/c69862c1-9adf-4a07-8f9a-5dada3826a6c)

## Groups management

In our example the user `authd test` is a member of the following Azure groups:

![image](https://github.com/ubuntu/authd/assets/1928546/5f994ae6-7473-4af3-8d1f-64c9dd2e10f8)

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

## Entra ID broker

The broker is managed through the ```snap``` command. 

The main operation is to restart the broker to reload the configuration when it has changed. You can reload the broker with the command:

```shell
snap restart authd-msentraid
```
