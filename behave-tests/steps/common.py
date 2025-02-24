import logging
import sys
import os
import subprocess
import xml.etree.ElementTree as ET

import libvirt
import behave.runner
from behave import *

# Add helpers module to the path
sys.path.append(os.path.join(os.path.dirname(__file__), "..", "helpers"))

import executil

logger = logging.getLogger(os.path.basename(__file__))

use_step_matcher("re")

MAIN_TEST_VM_NAME = "behave-test-vm"
MAIN_TEST_VM_DISK_SPACE = "5G"
SNAPSHOT_BOOTED_TO_GDM = "booted-to-gdm"

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
    disks = disk_devices_xml(ET.XML(domain.XMLDesc()))
    for disk in disks:
        target = disk.find("./target/@dev")
        if target is None:
            raise ValueError("Disk device does not have a target: %s" % ET.tostring(disk).decode())

        source = disk.find("./source/@file|./source/@dir|./source/@name|./source/@dev|./source/@volume")
        if source is None:
            continue

        pool = disk.find("./source/@pool")
        if pool:
            storage_pool = conn.storagePoolLookupByName(pool)
            volume = storage_pool.storageVolLookupByName(source)
        else:
            volume = conn.storageVolLookupByPath(source)

        if not volume:
            raise ValueError("Volume not found: %s" % source)

        volume.delete()

    # Undefine the domain
    domain.undefine_flags(libvirt.VIR_DOMAIN_UNDEFINE_MANAGED_SAVE | \
                          libvirt.VIR_DOMAIN_UNDEFINE_SNAPSHOTS_METADATA)

@given("I started Ubuntu Desktop")
def step_impl(context: behave.runner.Context):
    # Check if we can use a snapshot
    # if not context.config.userdata.getbool("FORCE_NEW_VMS") and has_snapshot(SNAPSHOT_BOOTED_TO_GDM):
    #     # Restore the initial snapshot
    #     executil.check_call(["multipass", "restore", f"{MAIN_TEST_VM_NAME}.{SNAPSHOT_BOOTED_TO_GDM}"])
    #     return

    if not context.config.userdata.getbool("FORCE_NEW_VMS") and has_snapshot(SNAPSHOT_BOOTED_TO_GDM):
        logging.debug("Restoring snapshot '%s'", SNAPSHOT_BOOTED_TO_GDM)
        conn = libvirt.open("qemu:///system")
        domain = conn.lookupByName(MAIN_TEST_VM_NAME)
        snapshot = domain.snapshotLookupByName(SNAPSHOT_BOOTED_TO_GDM)
        domain.revertToSnapshot(snapshot)
        return

    # There is no snapshot (or the user requested new VMs), so we need to prepare the VM.
    # First, ensure that the main test VM is purged
    ensure_vm_is_purged(MAIN_TEST_VM_NAME)

    # Launch a new main test VM
    executil.check_call(["multipass", "launch",
                         "--name", MAIN_TEST_VM_NAME,
                         "--disk", MAIN_TEST_VM_DISK_SPACE,
                         ])

    # Install the GNOME desktop
    executil.check_call(["multipass", "exec", MAIN_TEST_VM_NAME, "--", "sudo", "apt", "update"])
    executil.check_call(["multipass", "exec", MAIN_TEST_VM_NAME, "--", "sudo", "apt", "install", "-y", "gnome-shell"])

    # Detach the cloud-init disk
    conn = libvirt.open("qemu:///system")
    domain = conn.lookupByName(MAIN_TEST_VM_NAME)
    root = ET.fromstring(domain.XMLDesc())
    for disk in root.findall(".//devices/disk"):
        file = disk.find("source").get("file")
        if os.path.basename(file) == "cloud-init-config.iso":
            domain.detachDevice(ET.tostring(disk).decode())

    # Create the snapshot
    # executil.check_call(["multipass", "snapshot", MAIN_TEST_VM_NAME, "--name", INITIAL_SNAPSHOT_NAME])
    xml = internal_snapshot_xml(SNAPSHOT_BOOTED_TO_GDM, "Initial snapshot")
    domain.snapshotCreateXML(ET.tostring(xml).decode())


@step("I installed the authd package")
def step_impl(context: behave.runner.Context):
    host_path = context.config.userdata["authd_package"]
    vm_path = f"/tmp/{os.path.basename(host_path)}"
    executil.check_call(["multipass", "copy-files", host_path, f"{MAIN_TEST_VM_NAME}:{vm_path}"])
    executil.check_call(["multipass", "exec", MAIN_TEST_VM_NAME, "--", "sudo", "dpkg", "-i", vm_path])


@given("I installed the authd-msentraid snap")
def step_impl(context: behave.runner.Context):
    host_path = context.config.userdata["authd_msentraid_snap"]
    vm_path = f"/tmp/{os.path.basename(host_path)}"
    executil.check_call(["multipass", "copy-files", host_path, f"{MAIN_TEST_VM_NAME}:{vm_path}"])
    executil.check_call(["multipass", "exec", MAIN_TEST_VM_NAME, "--", "sudo", "snap", "install", "--dangerous", vm_path])


@when('I click on "Not listed\?"')
def step_impl(context: behave.runner.Context):
    raise NotImplementedError(u'STEP: When I click on "Not listed?"')


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
