import contextlib
import json
import subprocess
import time
from logging import getLogger
import os
import xml.etree.ElementTree as ET

import libvirt
from gi.repository import Gio, GLib
from screen import Screen
import accessible
import executil
from util import retry, RetriableError

logger = getLogger(os.path.basename(__file__))

WAIT_FOR_VM_STOPPED_TIMEOUT = 30
WAIT_FOR_VM_RUNNING_TIMEOUT = 30

VSOCK_CID = 3
MIN_A11Y_BUS_PROXY_VSOCK_PORT = 6000
MAX_A11Y_BUS_PROXY_VSOCK_PORT = 6100

RUNTIME_DIR = os.path.join(os.getenv("XDG_RUNTIME_DIR", "/run/user/%d" % os.getuid()), "behave-tests")

class VM:
    def __init__(self,
                 libvirt_connection: libvirt.virConnect,
                 name: str,
                 disk_size="5G",
                 memory="2G"):
        self.libvirt_connection = libvirt_connection
        self.name = name
        self.disk_size = disk_size
        self.memory = memory

        try:
            self.domain = libvirt_connection.lookupByName(name)  # type: libvirt.virDomain | None
        except libvirt.libvirtError:
            self.domain = None

        self.screen = Screen(self)
        self.runtime_dir = os.path.join(RUNTIME_DIR, name)
        self.a11y_bus_user = "gdm" # type: str|None

        global VSOCK_CID
        self.vsock_cid = VSOCK_CID
        # The next VM must use a different CID
        VSOCK_CID += 1

        self._cache = {}

    def launch(self):
        logger.debug("Launching VM '%s'", self.name)
        executil.check_call(["multipass", "launch",
                             "--name", self.name,
                             "--disk", self.disk_size,
                             "--memory", self.memory])
        # Ensure that self.domain is set
        self.domain = self.libvirt_connection.lookupByName(self.name)

    def start(self):
        logger.debug("Starting VM '%s'", self.name)
        executil.check_call(["multipass", "start", self.name])
        self.wait_until_running()

    def stop(self):
        logger.debug("Stopping VM '%s'", self.name)
        executil.check_call(["multipass", "stop", self.name])
        self.wait_until_stopped()
        # The cached data is no longer valid
        self._cache = {}

    def restart(self):
        self.stop()
        self.start()

    def run(self, command: [str], *args, **kwargs):
        return executil.run(["multipass", "exec", self.name, "--"] + command, *args, **kwargs)

    def check_call(self, command: [str], *args, **kwargs):
        return executil.check_call(["multipass", "exec", self.name, "--"] + command, *args, **kwargs)

    def check_output(self, command: [str], *args, **kwargs) -> str:
        return executil.check_output(["multipass", "exec", self.name, "--"] + command, *args, **kwargs)

    def set_clipboard(self, text: str):
        self.check_call(["wl-copy", text])

    def get_ip(self) -> str:
        vm_info_json = executil.check_output(["multipass", "info", self.name, "--format", "json"])
        vm_info = json.loads(vm_info_json)
        ip = vm_info["info"][self.name]["ipv4"][0]
        return ip

    def wait_until_stopped(self):
        logger.debug("Waiting for VM '%s' to stop", self.name)
        def check_vm_stopped():
            logger.debug("Checking if VM '%s' is stopped", self.name)
            try:
                vm_info_json = executil.check_output(["multipass", "info", self.name, "--format", "json"])
            except subprocess.CalledProcessError as e:
                if e.returncode == 2:
                    # The VM does not exist
                    raise RetriableError("The VM does not exist yet")
                else:
                    # Unexpected error
                    raise

            vm_info = json.loads(vm_info_json)
            if vm_info["info"][self.name]["state"] != "Stopped":
                raise RetriableError("The VM is not stopped yet")
        retry(check_vm_stopped, WAIT_FOR_VM_STOPPED_TIMEOUT, 1)

    def wait_until_running(self):
        start_time = time.monotonic()
        while time.monotonic() - start_time < WAIT_FOR_VM_RUNNING_TIMEOUT:
            try:
                vm_info_json = executil.check_output(["multipass", "info", self.name, "--format", "json"])
            except subprocess.CalledProcessError as e:
                if e.returncode == 2:
                    # The VM does not exist
                    time.sleep(1)
                    continue
                else:
                    # Unexpected error
                    raise

            vm_info = json.loads(vm_info_json)
            if vm_info["info"][self.name]["state"] == "Running":
                return

            # Try executing a command in the VM to check if it's running
            try:
                self.check_call(["true"])
            except subprocess.CalledProcessError:
                time.sleep(1)
                continue

        raise TimeoutError(f"VM '{self.name}' did not start within the timeout ({WAIT_FOR_VM_RUNNING_TIMEOUT} seconds)")

    def is_running(self) -> bool:
        return self.domain.isActive()

    def has_snapshot(self, snapshot_name) -> bool:
        if not self.domain:
            return False

        try:
            self.domain.snapshotLookupByName(snapshot_name)
            return True
        except libvirt.libvirtError:
            return False

    def restore_snapshot(self, snapshot_name):
        logger.debug("Restoring snapshot '%s' of VM '%s'", snapshot_name, self.name)
        snapshot = self.domain.snapshotLookupByName(snapshot_name)
        self.domain.revertToSnapshot(snapshot)

        if not self.is_running():
            return

        # Synchronize the time in the VM
        self.run(["sudo", "systemctl", "restart", "systemd-timesyncd"])

    def create_snapshot(self, name: str, description: str):
        self.detach_cloud_init_disk()
        xml = self.internal_snapshot_xml(name, description)
        self.domain.snapshotCreateXML(ET.tostring(xml).decode())

    def internal_snapshot_xml(self, name, description) -> ET.Element:
        res = ET.Element("domainsnapshot")
        name_elem = ET.SubElement(res, "name")
        name_elem.text = name
        description_elem = ET.SubElement(res, "description")
        description_elem.text = description

        # Get the disks of the domain
        disks = self.list_disk_devices()

        disks_elem = ET.SubElement(res, "disks")
        for dev in disks:
            ET.SubElement(disks_elem, "disk", name=dev, snapshot="internal")

        return res

    def detach_cloud_init_disk(self):
        root = ET.fromstring(self.domain.XMLDesc())
        for disk in root.findall(".//devices/disk"):
            file = disk.find("source").get("file")
            if os.path.basename(file) == "cloud-init-config.iso":
                self.domain.detachDevice(ET.tostring(disk).decode())

    def list_disk_devices(self) -> list:
        ret = []
        for e in ET.XML(self.domain.XMLDesc()).findall(".//devices/disk"):
            ret.append(e.find("target").get("dev"))
        return ret

    def define_devices(self):
        # Remove all video devices
        root = ET.fromstring(self.domain.XMLDesc())
        for device in root.findall(".//devices/video"):
            root.find("devices").remove(device)

        # Attach a virtio video device
        logger.debug("Attaching a virtio video device to VM '%s'", self.name)
        video = ET.Element("video")
        ET.SubElement(video, "model", type="virtio", primary="yes")
        root.find("devices").append(video)

        # Remove all graphics devices
        for device in root.findall(".//devices/graphics"):
            root.find("devices").remove(device)

        # Attach a Spice display
        # <graphics type="spice" port="5902" autoport="yes" listen="127.0.0.1">
        #   <listen type="address" address="127.0.0.1"/>
        #   <image compression="off"/>
        #   <gl enable="no"/>
        # </graphics>
        logger.debug("Attaching a Spice display to VM '%s'", self.name)
        graphics = ET.Element("graphics", type="spice", port="5902", autoport="yes", listen="127.0.0.1")
        ET.SubElement(graphics, "listen", type="address", address="127.0.0.1")
        ET.SubElement(graphics, "image", compression="off")
        ET.SubElement(graphics, "gl", enable="no")
        root.find("devices").append(graphics)

        # Attach a spice channel
        # <channel type="spicevmc">
        #   <target type="virtio" name="com.redhat.spice.0" state="disconnected"/>
        #   <alias name="channel0"/>
        #   <address type="virtio-serial" controller="0" bus="0" port="1"/>
        # </channel>
        has_spice_channel = False
        for channel in root.findall(".//devices/channel"):
            if channel.get("type") == "spicevmc":
                has_spice_channel = True
                break

        if not has_spice_channel:
            logger.debug("Attaching a Spice channel to VM '%s'", self.name)
            channel = ET.Element("channel", type="spicevmc")
            ET.SubElement(channel, "target", type="virtio", name="com.redhat.spice.0", state="disconnected")
            ET.SubElement(channel, "alias", name="channel0")
            ET.SubElement(channel, "address", type="virtio-serial", controller="0", bus="0", port="1")
            root.find("devices").append(channel)

        # Attach a vsock device
        #  <vsock model='virtio'>
        #    <cid auto='no' address='3'/>
        #  </vsock>
        logger.debug("Attaching a vsock device to VM '%s'", self.name)
        vsock = ET.Element("vsock", model="virtio")
        ET.SubElement(vsock, "cid", auto="no", address=str(self.vsock_cid))
        root.find("devices").append(vsock)

        # Re-define the domain
        self.domain = self.libvirt_connection.defineXMLFlags(ET.tostring(root).decode(), libvirt.VIR_DOMAIN_DEFINE_VALIDATE)

    def ensure_is_purged(self):
        # First, check if multipass knows about the VM
        exists_in_multipass = False
        try:
            executil.check_call(["multipass", "info", self.name],
                                stdout=subprocess.DEVNULL,
                                stderr=subprocess.DEVNULL)
            exists_in_multipass = True
        except subprocess.CalledProcessError:
            pass

        # Ensure that the VM is purged both in multipass and libvirt (deleting it in
        # multipass does not delete it in libvirt)
        try:
            executil.check_call(["multipass", "delete", "--purge", self.name])
        except subprocess.CalledProcessError:
            if exists_in_multipass:
                # Purging the VM in multipass failed, but it exists there
                raise

        # Check if the VM exists in libvirt
        conn = libvirt.open("qemu:///system")
        try:
            domain = conn.lookupByName(self.name)
        except libvirt.libvirtError:
            # The domain does not exist
            return

        # Delete all disks
        # disks = disk_devices_xml(ET.XML(domain.XMLDesc()))
        # for disk in disks:
        #     source = disk.find("./source/@file|./source/@dir|./source/@name|./source/@dev|./source/@volume")
        #     if source is None:
        #         continue
        #
        #     pool = disk.find("./source/@pool")
        #     if pool:
        #         storage_pool = conn.storagePoolLookupByName(pool)
        #         volume = storage_pool.storageVolLookupByName(source)
        #     else:
        #         volume = conn.storageVolLookupByPath(source)
        #
        #     if not volume:
        #         raise ValueError("Volume not found: %s" % source)
        #
        #     volume.delete()

        # Undefine the domain
        domain.undefineFlags(libvirt.VIR_DOMAIN_UNDEFINE_MANAGED_SAVE | \
                             libvirt.VIR_DOMAIN_UNDEFINE_SNAPSHOTS_METADATA)

    @property
    def a11y_bus_proxy(self) -> Gio.DBusProxy:
        cache_key = f"a11y-bus-proxy-{self.a11y_bus_user}"
        if cache_key in self._cache:
            return self._cache[cache_key]
        self._cache[cache_key] = self._forward_a11y_bus()
        return self._cache[cache_key]

    def _forward_a11y_bus(self) -> Gio.DBusProxy:
        uid = self.check_output(["id", "-u", self.a11y_bus_user]).strip()
        host_socket_path = os.path.join(self.runtime_dir, f"a11y-bus-{uid}")
        host_unit_name = f"a11y-bus-proxy-{self.name}-{uid}"
        vm_unit_name = f"a11y-bus-proxy-{uid}"
        vsock_port = self.find_free_a11y_bus_proxy_vsock_port()
        if vsock_port is None:
            raise RuntimeError("No free vsock port found")

        # Wait for the accessibility bus to be available
        def check_a11y_bus_available():
            logger.debug("Checking if the a11y bus is available")
            try:
                self.check_call(
                    ["sudo", "ls", f"/run/user/{uid}/at-spi/bus"])
            except subprocess.CalledProcessError:
                raise RetriableError("The bus is not available yet")

        retry(check_a11y_bus_available, 30, 0.2)

        # Forward the vsock port in the VM to the a11y bus
        self.check_call(["sudo", "systemd-run", "--unit", vm_unit_name, "--",
                         "socat",
                         f"VSOCK-LISTEN:{vsock_port},reuseaddr,fork",
                         f"UNIX-CONNECT:/run/user/{uid}/at-spi/bus"])

        # Forward the a11y-bus Unix socket on the host to the vsock port
        os.makedirs(os.path.dirname(host_socket_path), exist_ok=True)
        if os.path.exists(host_socket_path):
            os.unlink(host_socket_path)

        # Reset the unit if it failed. We don't care about the exit code, so we
        # use executil.run().
        def try_resetting_unit():
            logger.debug("Trying to reset the unit")
            executil.run(
                ["systemctl", "--user", "stop", "--force", host_unit_name],
                stderr=subprocess.DEVNULL)
            executil.run(["systemctl", "--user", "reset-failed", "--force",
                          host_unit_name], stderr=subprocess.DEVNULL)
            try:
                executil.check_call(
                    ["systemctl", "--user", "status", host_unit_name])
                raise RetriableError("The unit is still running")
            except subprocess.CalledProcessError as e:
                if e.returncode == 4:
                    # The unit is unknown, which is what we want
                    return
                # All other exit codes mean that the unit is still known
                raise RetriableError("The unit is still known")

        retry(try_resetting_unit, 5, 0.2)

        # Run the proxy
        executil.check_call(
            ["systemd-run", "--user", "--unit", host_unit_name, "--",
             "socat", f"UNIX-LISTEN:{host_socket_path},fork",
             f"VSOCK-CONNECT:{self.vsock_cid}:{vsock_port}"])

        logger.info("Connecting to bus %s", host_socket_path)

        def try_connecting_to_a11y_bus():
            try:
                return Gio.DBusConnection.new_for_address_sync(
                    address=f"unix:path={host_socket_path}",
                    flags=Gio.DBusConnectionFlags.AUTHENTICATION_CLIENT |
                          Gio.DBusConnectionFlags.MESSAGE_BUS_CONNECTION,
                    observer=None,
                    cancellable=None
                )
            except GLib.Error as e:
                if e.code == Gio.IOErrorEnum.NOT_FOUND:
                    raise RetriableError("The bus is not available yet") from e
                raise

        return retry(try_connecting_to_a11y_bus, 5, 0.2,
                     "Failed to connect to the a11y bus")

    def find_free_a11y_bus_proxy_vsock_port(self) -> int|None:
            return self.find_free_vsock_port(MIN_A11Y_BUS_PROXY_VSOCK_PORT, MAX_A11Y_BUS_PROXY_VSOCK_PORT)

    def find_free_vsock_port(self, min_port, max_port) -> int|None:
        for port in range(min_port, max_port + 1):
            try:
                self.check_call(["timeout", "1s", "sudo", "socat", f"VSOCK-LISTEN:{port}", "echo"], stderr=subprocess.DEVNULL)
            except subprocess.CalledProcessError as e:
                if e.returncode == 124:
                    # socat timed out
                    return port

        return None

    @property
    def gnome_shell(self) -> accessible.Root:
        return self.application("gnome-shell")

    def application(self, app_name) -> accessible.Root:
        cache_key = f"application-{self.a11y_bus_user}-{app_name}"
        if cache_key in self._cache:
            return self._cache[cache_key]

        self._cache[cache_key] = accessible.application_root(self.a11y_bus_proxy, app_name)
        return self._cache[cache_key]
