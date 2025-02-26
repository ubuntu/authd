import json
import logging
import sys
import os
import subprocess
import time
import xml.etree.ElementTree as ET

import libvirt
import behave.runner
from behave import *
from gi.repository import Gio

# Add helpers module to the path
sys.path.append(os.path.join(os.path.dirname(__file__), "..", "helpers"))

import executil

logger = logging.getLogger(os.path.basename(__file__))

use_step_matcher("re")

MAIN_TEST_VM_NAME = "behave-test-vm"
MAIN_TEST_VM_DISK_SPACE = "5G"
BASE_SNAP = "core24"
WAIT_FOR_VM_STOPPED_TIMEOUT = 30
WAIT_FOR_VM_RUNNING_TIMEOUT = 30
SNAPSHOT_INSTALLED_GNOME_SESSION = "installed-gnome-session"
SNAPSHOT_BASE = "base-snapshot"
SSH_PRIVATE_KEYFILE = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", ".ssh-key"))
SSH_PUBLIC_KEYFILE = SSH_PRIVATE_KEYFILE + ".pub"
RUNTIME_DIR = os.path.join(os.getenv("XDG_RUNTIME_DIR", "/run/user/%d" % os.getuid()), "behave-tests")

# TODO: Install the dogtail service from GitHub or PyPI
DOGTAIL_SERVICE_PATH = os.path.expanduser("~/projects/dogtail-service/dogtail-service")
DOGTAIL_SERVICE_LAUNCHER_PATH = os.path.expanduser("~/projects/dogtail-service/dogtail-service-launcher")
DOGTAIL_SERVICE_DBUS_SERVICE_FILE = os.path.expanduser("~/projects/dogtail-service/com.ubuntu.DesktopQA.Dogtail.service")
DOGTAIL_SERVICE_SYSTEMD_SERVICE_FILE = os.path.expanduser("~/projects/dogtail-service/dogtail-service.service")

def start_vm(name: str):
    logging.debug("Starting VM '%s'", name)
    executil.check_call(["multipass", "start", name])
    wait_until_vm_is_running(name)

def stop_vm(name: str):
    logging.debug("Stopping VM '%s'", name)
    executil.check_call(["multipass", "stop", name])
    wait_until_vm_is_stopped(name)

def restart_vm(name: str):
    stop_vm(name)
    start_vm(name)

def wait_until_vm_is_stopped(name: str):
    start_time = time.monotonic()
    while time.monotonic() - start_time < WAIT_FOR_VM_STOPPED_TIMEOUT:
        try:
            vm_info_json = executil.check_output(["multipass", "info", name, "--format", "json"])
        except subprocess.CalledProcessError as e:
            logging.debug("multipass info failed: %s", e)
            time.sleep(1)
            continue

        vm_info = json.loads(vm_info_json)
        if vm_info["info"][name]["state"] == "Stopped":
            return

        time.sleep(1)

    raise TimeoutError("The VM did not stop within the timeout (%d seconds)" % WAIT_FOR_VM_STOPPED_TIMEOUT)

def wait_until_vm_is_running(name: str):
    start_time = time.monotonic()
    while time.monotonic() - start_time < WAIT_FOR_VM_RUNNING_TIMEOUT:
        try:
            vm_info_json = executil.check_output(["multipass", "info", name, "--format", "json"])
        except subprocess.CalledProcessError as e:
            logging.debug("multipass info failed: %s", e)
            time.sleep(1)
            continue

        vm_info = json.loads(vm_info_json)
        if vm_info["info"][name]["state"] == "Running":
            return

        time.sleep(1)

    raise TimeoutError("The VM did not start within the timeout (%d seconds)" % WAIT_FOR_VM_RUNNING_TIMEOUT)


def ensure_ssh_key():
    if not os.path.exists(SSH_PRIVATE_KEYFILE):
        executil.check_call(["ssh-keygen", "-t", "rsa", "-b", "4096", "-N", "", "-f", SSH_PRIVATE_KEYFILE])

# def has_snapshot(snapshot_name) -> bool:
#     try:
#         executil.check_call(["multipass", "info", f"{MAIN_TEST_VM_NAME}.{snapshot_name}"],
#                             stderr=subprocess.DEVNULL)
#         return True
#     except subprocess.CalledProcessError:
#         return False

def has_snapshot(snapshot_name) -> bool:
    conn = libvirt.open("qemu:///system")
    try:
        domain = conn.lookupByName(MAIN_TEST_VM_NAME)
        domain.snapshotLookupByName(snapshot_name)
        return True
    except libvirt.libvirtError:
        return False

def detach_cloud_init_disk(domain: libvirt.virDomain):
    root = ET.fromstring(domain.XMLDesc())
    for disk in root.findall(".//devices/disk"):
        file = disk.find("source").get("file")
        if os.path.basename(file) == "cloud-init-config.iso":
            domain.detachDevice(ET.tostring(disk).decode())

  # def list_disk_devs
  #   ret = []
  #   domain_xml.elements.each('domain/devices/disk') do |e|
  #     ret << e.elements['target'].attribute('dev').to_s
  #   end
  #   ret
  # end

def list_disk_devices(domain_xml) -> list:
    ret = []
    for e in domain_xml.findall(".//devices/disk"):
        ret.append(e.find("target").get("dev"))
    return ret

  # def disk_type(dev)
  #   domain_xml.elements.each('domain/devices/disk') do |e|
  #     if e.elements['target'].attribute('dev').to_s == dev
  #       return e.elements['driver'].attribute('type').to_s
  #     end
  #   end
  #   raise "No such disk device '#{dev}'"
  # end

def disk_devices_xml(domain_xml: ET.Element) -> list[ET.Element]:
    ret = []
    for e in domain_xml.findall(".//devices/disk"):
        ret.append(e)
    return ret

def disk_type(domain_xml, dev) -> str:
    for e in domain_xml.findall(".//devices/disk"):
        if e.find("target").get("dev") == dev:
            return e.find("driver").get("type")
    raise ValueError(f"No such disk device '{dev}'")

  # def internal_snapshot_xml(name)
  #   disk_devs = list_disk_devs
  #   disks_xml = "    <disks>\n"
  #   disk_devs.each do |dev|
  #     snapshot_type = disk_type(dev) == 'qcow2' ? 'internal' : 'no'
  #     disks_xml +=
  #       "      <disk name='#{dev}' snapshot='#{snapshot_type}'></disk>\n"
  #   end
  #   disks_xml += '    </disks>'
  #   <<~XML
  #     <domainsnapshot>
  #       <name>#{name}</name>
  #       <description>Snapshot for #{name}</description>
  #     #{disks_xml}
  #       </domainsnapshot>
  #   XML
  # end

def create_snapshot(domain: libvirt.virDomain, name: str, description: str):
    detach_cloud_init_disk(domain)
    xml = internal_snapshot_xml(name, description)
    domain.snapshotCreateXML(ET.tostring(xml).decode())

def internal_snapshot_xml(name, description) -> ET.Element:
    res = ET.Element("domainsnapshot")
    name_elem = ET.SubElement(res, "name")
    name_elem.text = name
    description_elem = ET.SubElement(res, "description")
    description_elem.text = description

    # Get the disks of the domain
    conn = libvirt.open("qemu:///system")
    domain = conn.lookupByName(MAIN_TEST_VM_NAME)
    disks = list_disk_devices(ET.XML(domain.XMLDesc()))

    disks_elem = ET.SubElement(res, "disks")
    for dev in disks:
        ET.SubElement(disks_elem, "disk", name=dev, snapshot="internal")

    return res

def ensure_vm_is_purged(name: str):
    # First, check if multipass knows about the VM
    exists_in_multipass = False
    try:
        executil.check_call(["multipass", "info", name], stderr=subprocess.DEVNULL)
        exists_in_multipass = True
    except subprocess.CalledProcessError:
        pass

    # Ensure that the VM is purged both in multipass and libvirt (deleting it in
    # multipass does not delete it in libvirt)
    try:
        executil.check_call(["multipass", "delete", "--purge", name])
    except subprocess.CalledProcessError:
        if exists_in_multipass:
            # Purging the VM in multipass failed, but it exists there
            raise

    # Check if the VM exists in libvirt
    conn = libvirt.open("qemu:///system")
    try:
        domain = conn.lookupByName(name)
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

@given("I have a Ubuntu Desktop system")
def step_impl(context: behave.runner.Context):
    # Check if we can use a snapshot
    # if not context.config.userdata.getbool("FORCE_NEW_VMS") and has_snapshot(SNAPSHOT_BOOTED_TO_GDM):
    #     # Restore the initial snapshot
    #     executil.check_call(["multipass", "restore", f"{MAIN_TEST_VM_NAME}.{SNAPSHOT_BOOTED_TO_GDM}"])
    #     return

    if not context.config.userdata.getbool("FORCE_NEW_VMS") and has_snapshot(SNAPSHOT_BASE):
        logging.debug("Restoring snapshot '%s'", SNAPSHOT_BASE)
        conn = libvirt.open("qemu:///system")
        domain = conn.lookupByName(MAIN_TEST_VM_NAME)
        snapshot = domain.snapshotLookupByName(SNAPSHOT_BASE)
        domain.revertToSnapshot(snapshot)
        return

    if not context.config.userdata.getbool("FORCE_NEW_VMS") and has_snapshot(SNAPSHOT_INSTALLED_GNOME_SESSION):
        # Restore the snapshot with the installed GNOME session
        conn = libvirt.open("qemu:///system")
        domain = conn.lookupByName(MAIN_TEST_VM_NAME)
        snapshot = domain.snapshotLookupByName(SNAPSHOT_INSTALLED_GNOME_SESSION)
        domain.revertToSnapshot(snapshot)
    else:
        # There is no snapshot (or the user requested new VMs), so we need to prepare the VM.
        # First, ensure that the main test VM is purged
        ensure_vm_is_purged(MAIN_TEST_VM_NAME)

        # Launch a new main test VM
        executil.check_call(["multipass", "launch",
                             "--name", MAIN_TEST_VM_NAME,
                             "--disk", MAIN_TEST_VM_DISK_SPACE,
                             ])

        # Add the authd PPA (to install gnome-shell and yaru-theme-gnome-shell from the PPA)
        executil.check_call(["multipass", "exec", MAIN_TEST_VM_NAME, "--",
                             "sudo", "add-apt-repository", "-y", "ppa:ubuntu-enterprise-desktop/authd"])

        # Install the GNOME desktop
        executil.check_call(["multipass", "exec", MAIN_TEST_VM_NAME, "--", "sudo", "apt", "update"])
        executil.check_call(["multipass", "exec", MAIN_TEST_VM_NAME, "--", "sudo", "apt", "install", "-y", "gnome-session"])

        # Create the snapshot
        conn = libvirt.open("qemu:///system")
        domain = conn.lookupByName(MAIN_TEST_VM_NAME)
        create_snapshot(domain, SNAPSHOT_INSTALLED_GNOME_SESSION, "Installed GNOME session")


    # Install authd and the authd-msentraid snap
    # TODO: This should not be done here
    executil.check_call(["multipass", "exec", MAIN_TEST_VM_NAME, "--", "sudo", "apt", "install", "-y", "authd"])
    executil.check_call(["multipass", "exec", MAIN_TEST_VM_NAME, "--", "sudo", "snap", "install", "authd-msentraid"])

    # Install the dogtail service
    executil.check_call(["multipass", "transfer", DOGTAIL_SERVICE_PATH, f"{MAIN_TEST_VM_NAME}:/usr/local/bin/dogtail-service"])
    executil.check_call(["multipass", "transfer", DOGTAIL_SERVICE_LAUNCHER_PATH, f"{MAIN_TEST_VM_NAME}:/usr/bin/dogtail-service"])
    executil.check_call(["multipass", "transfer", DOGTAIL_SERVICE_DBUS_SERVICE_FILE, f"{MAIN_TEST_VM_NAME}:/usr/share/dbus-1/services/"])
    executil.check_call(["multipass", "transfer", DOGTAIL_SERVICE_SYSTEMD_SERVICE_FILE, f"{MAIN_TEST_VM_NAME}:/etc/systemd/session/"])
    executil.check_call(["multipass", "exec", MAIN_TEST_VM_NAME, "--", "sudo", "systemctl", "daemon-reload"])
    executil.check_call(["multipass", "exec", MAIN_TEST_VM_NAME, "--", "sudo", "systemctl", "enable", "dogtail-service"])

    # Enable anonymous authentication to the a11y bus (external authentication would
    # require that the user connecting to the forwarded a11y bus on the host has the
    # same UID as the user in the VM).
    logging.debug("Enabling anonymous authentication to the a11y bus")
    old_config = "<auth>EXTERNAL</auth>"
    new_config = "<auth>ANONYMOUS</auth>\\n  " \
                 "<allow_anonymous/>\\n  " \
                 "<listen>tcp:host=localhost,bind=0.0.0.0,port=0</listen>"
    executil.check_call(["multipass", "exec", MAIN_TEST_VM_NAME, "--",
                         "sudo", "sed", "-i", f"s|{old_config}|{new_config}|",
                        "/usr/share/defaults/at-spi2/accessibility.conf"])

    # Copy the SSH key
    logging.debug("Copying the SSH key to the VM")
    ensure_ssh_key()
    vm_public_keyfile = f"/tmp/id_behave_tests.pub"
    executil.check_call(["multipass", "transfer", SSH_PUBLIC_KEYFILE, f"{MAIN_TEST_VM_NAME}:{vm_public_keyfile}"])
    executil.check_call(["multipass", "exec", MAIN_TEST_VM_NAME, "--",
                         "sudo", "sh", "-c", f"cat {vm_public_keyfile} >> /home/ubuntu/.ssh/authorized_keys"])
    executil.check_call(["multipass", "exec", MAIN_TEST_VM_NAME, "--",
                         "sudo", "sh", "-c", f"cat {vm_public_keyfile} >> /root/.ssh/authorized_keys"])

    # Stop the VM
    stop_vm(MAIN_TEST_VM_NAME)

    # Remove all video devices
    root = ET.fromstring(domain.XMLDesc())
    for device in root.findall(".//devices/video"):
        root.find("devices").remove(device)

    # Attach a virtio video device
    logging.debug("Attaching a virtio video device")
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
    logging.debug("Attaching a Spice display")
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
        logging.debug("Attaching a Spice channel")
        channel = ET.Element("channel", type="spicevmc")
        ET.SubElement(channel, "target", type="virtio", name="com.redhat.spice.0", state="disconnected")
        ET.SubElement(channel, "alias", name="channel0")
        ET.SubElement(channel, "address", type="virtio-serial", controller="0", bus="0", port="1")
        root.find("devices").append(channel)

    # Re-define the domain
    domain = conn.defineXMLFlags(ET.tostring(root).decode(), libvirt.VIR_DOMAIN_DEFINE_VALIDATE)

    start_vm(MAIN_TEST_VM_NAME)

    # Create the snapshot
    create_snapshot(domain, SNAPSHOT_BASE, "Initial snapshot")

@step("I installed the authd package")
def step_impl(context: behave.runner.Context):
    host_path = context.config.userdata["authd_package"]
    vm_path = f"/tmp/{os.path.basename(host_path)}"
    executil.check_call(["multipass", "copy-files", host_path, f"{MAIN_TEST_VM_NAME}:{vm_path}"])
    executil.check_call(["multipass", "exec", MAIN_TEST_VM_NAME, "--", "sudo", "dpkg", "-i", vm_path])


@step("I installed the authd-msentraid snap")
def step_impl(context: behave.runner.Context):
    host_path = context.config.userdata["authd_msentraid_snap"]
    vm_path = f"/tmp/{os.path.basename(host_path)}"
    executil.check_call(["multipass", "copy-files", host_path, f"{MAIN_TEST_VM_NAME}:{vm_path}"])
    executil.check_call(["multipass", "exec", MAIN_TEST_VM_NAME, "--", "sudo", "snap", "install", "--dangerous", vm_path])

@step("I configured authd to use the MS Entra ID broker")
def step_impl(context: behave.runner.Context):
    src = "/snap/authd-msentraid/current/conf/authd/msentraid.conf"
    dest = "/etc/authd/brokers.d/"
    executil.check_call(["multipass", "exec", MAIN_TEST_VM_NAME, "--", "sudo", "install", "-D", "--target-directory", dest, src])

@step("I configured the MS Entra ID broker to use the test OIDC app")
def step_impl(context: behave.runner.Context):
    issuer_id = context.config.userdata["msentraid_issuer_id"]
    client_id = context.config.userdata["msentraid_client_id"]
    broker_config_file = "/var/snap/authd-msentraid/current/broker.conf"
    executil.check_call(["multipass", "exec", MAIN_TEST_VM_NAME, "--",
                         "sudo", "sed", "-i", "-e", f"s/<ISSUER_ID>/{issuer_id}/", "-e", f"s/<CLIENT_ID>/{client_id}/",
                         broker_config_file])


@step("I rebooted the system")
def step_impl(context: behave.runner.Context):
    restart_vm(MAIN_TEST_VM_NAME)


@step("I'm at the GDM login screen")
def step_impl(context: behave.runner.Context):
    # Get the IP address of the VM
    vm_info_json = executil.check_output(["multipass", "info", MAIN_TEST_VM_NAME, "--format", "json"])
    vm_info = json.loads(vm_info_json)
    logging.debug("VM info: %s", vm_info)
    ip = vm_info["info"][MAIN_TEST_VM_NAME]["ipv4"][0]

    # # Forward the GDM a11y bus
    # gdm_uid = executil.check_output(["multipass", "exec", MAIN_TEST_VM_NAME, "--", "id", "-u", "gdm"]).strip()
    # a11y_bus_host_path = os.path.join(RUNTIME_DIR, MAIN_TEST_VM_NAME, "a11y-bus")
    # os.makedirs(os.path.dirname(a11y_bus_host_path), exist_ok=True)
    # if os.path.exists(a11y_bus_host_path):
    #     os.unlink(a11y_bus_host_path)
    # subprocess.Popen(["ssh", "-i", SSH_PRIVATE_KEYFILE, "-N",
    #                   "-L", f"{a11y_bus_host_path}:/run/user/{gdm_uid}/at-spi/bus",
    #                   # Don't check if the host key is known
    #                   "-o", "StrictHostKeyChecking=no",
    #                   # Don't save the host key in the known hosts file
    #                   "-o", "UserKnownHostsFile=/dev/null",
    #                   f"root@{ip}"])
    #
    # logging.info("Connecting to bus %s", a11y_bus_host_path)
    # connection = Gio.DBusConnection.new_for_address_sync(
    #     address=f"unix:path={a11y_bus_host_path}",
    #     flags=Gio.DBusConnectionFlags.AUTHENTICATION_CLIENT |
    #           Gio.DBusConnectionFlags.MESSAGE_BUS_CONNECTION,
    #           # Gio.DBusConnectionFlags.CROSS_NAMESPACE,
    #     observer=None,
    #     cancellable=None
    # )

    # Forward the

    # Drop into a debug shell
    import ipdb; ipdb.set_trace()


@when('I click on "Not listed\?"')
def step_impl(context: behave.runner.Context):
    raise NotImplementedError(u'STEP: When I click on "Not listed\?"')



@step('I enter the UPN of the test user in the "Username" field')
def step_impl(context: behave.runner.Context):
    raise NotImplementedError(u'STEP: And I enter the UPN of the test user in the "Username" field')


@step('I press "Enter"')
def step_impl(context: behave.runner.Context):
    raise NotImplementedError(u'STEP: And I press "Enter"')


@then("I am asked to select the broker")
def step_impl(context: behave.runner.Context):
    raise NotImplementedError(u'STEP: Then I am asked to select the broker')


@when('I select the "Microsoft Entra ID" broker')
def step_impl(context: behave.runner.Context):
    raise NotImplementedError(u'STEP: When I select the "Microsoft Entra ID" broker')


@then(
    'I see the message "Scan the QR code or access "https://microsoft\.com/devicelogin" and use the provided login code"')
def step_impl(context: behave.runner.Context):
    raise NotImplementedError(
        u'STEP: Then I see the message "Scan the QR code or access "https://microsoft.com/devicelogin" and use the provided login code"')


@step("I see a QR code")
def step_impl(context: behave.runner.Context):
    raise NotImplementedError(u'STEP: And I see a QR code')


@step("I see a login code")
def step_impl(context: behave.runner.Context):
    raise NotImplementedError(u'STEP: And I see a login code')


@when('I open "https://microsoft\.com/devicelogin" on another machine and log in')
def step_impl(context: behave.runner.Context):
    raise NotImplementedError(u'STEP: When I open "https://microsoft.com/devicelogin" on another machine and log in')


@then("I am prompted to create a local password")
def step_impl(context: behave.runner.Context):
    raise NotImplementedError(u'STEP: Then I am prompted to create a local password')


@when("I enter a password")
def step_impl(context: behave.runner.Context):
    raise NotImplementedError(u'STEP: When I enter a password')


@step("confirm the password")
def step_impl(context: behave.runner.Context):
    raise NotImplementedError(u'STEP: And confirm the password')


@then("I am logged in")
def step_impl(context: behave.runner.Context):
    raise NotImplementedError(u'STEP: Then I am logged in')

