---
myst:
  html_meta:
    "description lang=en": "To use authd with LXD, you need to configure the UID and GID ranges."
---

(howto::use-with-lxd)=
# Use authd in LXD containers

Running authd inside a LXD container requires the configuration of UID and GID
ranges.

```{important}
These steps are in addition to the general installation and configuration steps
outlined in the [configuring authd guide](configure-authd.md).
```
## Default ID map ranges

The ID map range for LXD is: `1000000:1000000000`.

The default range for authd exceeds those values: `1000000000:1999999999`.

This causes errors when authenticating from LXD containers.

## Configuration options for using authd with LXD

Two options for configuring the ID ranges are outlined below.

### 1. Configure ID ranges for the authd service

Change the default ranges so that they don't exceed those from the user namespace mappings, for example:

```{code-block} diff
:caption: /etc/authd/authd.yaml

-#UID_MIN: 1000000000
+#UID_MIN: 100000
-#UID_MAX: 1999999999
+#UID_MAX: 1000000000
-#GID_MIN: 1000000000
+#GID_MIN: 100000
-#GID_MAX: 1999999999
+#GID_MAX: 1000000000

```

### 2. Configure subordinate ID ranges on the host

The mappings that apply to LXD containers can be found in the following files on the host:

* `/etc/subuid`
* `/etc/subgid`

Configure the subordinate ID range in each file to include the default authd ID range (`1000000000:1999999999`), for example:

```{code-block} diff
:caption: /etc/subuid
-<your-user>:100000:65536
+<your-user>:1000000000:1999999999
```
