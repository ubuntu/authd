# Using authd with Samba

The UIDs and GIDs assigned to users and groups by authd are unique to each
machine. This means that when using authd with Samba, the UIDs and GIDs of users
and groups on the Samba server will not match those on the client machines,
which leads to permission issues.

To avoid these issues, you can use Samba with ID mapping. This ensures that the
UIDs and GIDs of users and groups are mapped correctly across all machines.

## Setting up Samba with ID Mapping

This guide will walk you through setting up a Samba server with ID mapping. The
user `alice` will be able to access a shared directory on the server from a
client machine.

---

### Steps for the Server

1. **Install Samba:**
   Install the Samba server package:

   ```bash
   sudo apt update
   sudo apt install samba
   ```

2. **Create the Shared Directory:**
   Create the directory to be shared and set ownership to the `alice` user:

   ```bash
   sudo mkdir -p /srv/samba/alice
   sudo chown alice:alice /srv/samba/alice
   ```

3. **Edit Samba Configuration:**
   Open the Samba configuration file:

   ```bash
   sudo editor /etc/samba/smb.conf
   ```

   Add the following section at the end of the file:

   ```ini
   [alice]
   path = /srv/samba/alice
   browsable = yes
   writable = yes
   valid users = alice
   ```

   - **Explanation:** This section defines a Samba share named `alice` located
   at `/srv/samba/alice`. It is visible to users on the network (`browsable`),
   allows writing (`writable`), and restricts access to the `alice` user (`valid
   users`).

4. **Create a Samba User for `alice`:**
   Add the `alice` user to the Samba database and set a password:

   ```bash
   sudo smbpasswd -a alice
   ```

   Follow the prompts to set the Samba password for the user.

5. **Restart Samba Service:**
   Restart the Samba service to apply the changes:

   ```bash
   sudo systemctl restart smbd
   ```

---

### Steps for the Client

1. **Install Samba Client:**
   Install the required packages for connecting to Samba shares:

   ```bash
   sudo apt update
   sudo apt install smbclient cifs-utils
   ```

2. **Test Access to the Share:**
   Test connectivity using `smbclient` (replace `$SERVER` with the Samba
   server's hostname or IP address):

   ```bash
   smbclient //$SERVER/alice -U alice
   ```

   Enter the Samba password for `alice` when prompted. If successful, a `smb: \>`
   prompt appears.

3. **Mount the Share:**
   Create a mount point for the share:

   ```bash
   mkdir -p /home/alice/samba
   ```

   Mount the share using the `cifs` filesystem type:

   ```bash
   sudo mount -t cifs //$SERVER/alice /home/alice/samba -o user=alice,uid=$(id -u alice),gid=$(id -g alice)
   ```

   Enter the Samba password for `alice` when prompted.

4. **Optional: Add to `/etc/fstab` for Persistent Mounting:**
   To automatically mount the share at boot, use a credentials file:

   - Create a credentials file:

     ```bash
     sudo editor /etc/samba/credentials
     ```

     Add the following lines:

     ```
     username=alice
     password=YOUR_PASSWORD
     ```

   - Secure the credentials file:

     ```bash
     sudo chmod 600 /etc/samba/credentials
     ```

   - Update `/etc/fstab`:

     ```
     //$SERVER/alice /home/alice/samba cifs credentials=/etc/samba/credentials,uid=alice,gid=alice 0 0
     ```

5. **Verify the Mount:**
   As the user `alice`, try accessing the shared directory:

   ```bash
   ls -la /home/alice/samba
   ```

   Verify write access by creating a test file:

   ```bash
   touch /home/alice/samba/test
   ```

   **Security Note:** Files and directories in the share may appear as owned by
   `alice` on the client, but access control is enforced by the server. For
   example:

   - If `alice` does not have permission on the server, access will be denied
     even if ownership appears correct on the client.

   Example:

   - Create a restricted directory on the server:

     ```bash
     sudo mkdir /srv/samba/alice/secrets
     sudo chmod 700 /srv/samba/alice/secrets
     ```

   - Attempt to access it on the client:

     ```bash
     ls /home/alice/samba/secrets
     ```

     **Result:** *Permission denied.*

---

### Cleanup

#### On the Server

1. **Delete the Shared Directory:**
   Remove the directory used for the Samba share:

   ```bash
   sudo rm -rf /srv/samba/alice
   ```

2. **Purge Installed Samba Packages:**
   If Samba is no longer needed, uninstall it completely:

   ```bash
   sudo apt purge samba samba-common
   sudo apt autoremove
   ```

---

#### On the Client

1. **Unmount the Shared Directory:**
   Unmount the shared directory:

   ```bash
   sudo umount /home/alice/samba
   ```

2. **Delete the Mount Point:**
   Remove the mount point directory:

   ```bash
   rmdir /home/alice/samba
   ```

3. **Remove Entry from `/etc/fstab`:**
   If you added the share to `/etc/fstab`, remove its entry:

   ```bash
   sudo editor /etc/fstab
   ```

   Locate and delete the line referencing the Samba share, then save and close.

4. **Delete Credentials File:**
   If a credentials file was used, remove it:

   ```bash
   sudo rm /etc/samba/credentials
   ```

5. **Purge Installed Samba Client Packages:**
   If Samba client tools are no longer needed, uninstall them:

   ```bash
   sudo apt purge samba-common smbclient cifs-utils
   sudo apt autoremove
   ```
