# Cloud providers that authd supports

authd supports cloud providers through its identity brokers.
Each broker is available as a snap.
Several brokers can be installed and enabled on a system.

| Provider       | Broker snap                                             | Install as a snap             | Configure                                                           | Provider docs                                                            |
| ---            | ---                                                     | ---                           | ---                                                                 | ---                                                                      |
| Google IAM     | [authd-google](https://snapcraft.io/authd-google)       | `snap install authd-google`   | <a href="howto/install-authd/?broker=google">Google IAM guide</a>   | [Microsoft](https://learn.microsoft.com/en-us/entra/fundamentals/whatis) |
| MS Entra ID    | [authd-msentraid](https://snapcraft.io/authd-msentraid) | `snap install authd-msentraid`| <a href="howto/install-authd/?broker=google">MS Entra ID guide</a>  | [Google](https://cloud.google.com/iam/docs/overview)                     |

```{note}
Support for multiple additional providers is planned for future releases of authd.
```
