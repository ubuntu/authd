---
myst:
  html_meta:
    "description lang=en": "Deploy authd with Landscape."
---

# Reference deployment script

Landscape can be used to [remotely execute
scripts](https://documentation.ubuntu.com/landscape/how-to-guides/web-portal/web-portal-24-04-or-later/use-script-profiles/)
on client machines.

This page provides example snippets that can be used in your deployment scripts to
install and configure authd on Ubuntu machines.

```{note}
The page uses the Microsoft Entra ID broker, but the example can be extended to
other brokers.
```

## Setup

You need to have the following environmental variables defined:

```bash
ISSUER_ID=<ISSUER_ID>
CLIENT_ID=<CLIENT_ID>
SUFFIX_ALLOWED=<SUFFIX>
```

## Installation

To install the authd deb and the broker snap:

:::{literalinclude} ./code/deploy_authd/tests/deploy_authd_landscape/task.yaml
:language: text
:start-after: [docs:install-authd-and-broker]
:end-before: [docs:install-authd-and-broker-end]
:dedent: 2
:::

```{tip}
For more information on installing authd and its brokers, read the
[installation guide](../howto/install-authd.md).
```

## Configuration

To configure authd and the broker:

:::{literalinclude} ./code/deploy_authd/tests/deploy_authd_landscape/task.yaml
:language: text
:start-after: [docs:configure-authd-and-broker]
:end-before: [docs:configure-authd-and-broker-end]
:dedent: 2
:::

```{tip}
For more information on configuring authd, read the [configuration
guide](../howto/configure-authd.md).
```

## Restart the services

To restart the authd daemon, the broker snap, and the SSH service:

:::{literalinclude} ./code/deploy_authd/tests/deploy_authd_landscape/task.yaml
:language: bash
:start-after: [docs:restart-authd-and-broker]
:end-before: [docs:restart-authd-and-broker-end]
:dedent: 2
:::

## Authenticate with authd

Once the script is deployed, user login should be possible with authd.

For example, [using SSH](../howto/login-ssh.md):

```text
ssh <username>@<host>
```
