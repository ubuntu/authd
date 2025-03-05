import logging
import sys
import os
import tempfile

import libvirt
import behave.runner
from behave import *

use_step_matcher("re")

# Add helpers module to the path
sys.path.append(os.path.join(os.path.dirname(__file__), "..", "helpers"))

import checkpoints
import executil
import msentraid
from vm import VM

logger = logging.getLogger(os.path.basename(__file__))

use_step_matcher("re")

MAIN_TEST_VM_NAME = "behave-tests-main"
MAIN_TEST_VM_DISK_SPACE = "5G"
SECOND_TEST_VM_NAME = "behave-tests-second"
SECOND_TEST_VM_DISK_SPACE = "10G"
SNAPSHOT_BASE = "base-snapshot"

LIBVIRT_CONNECTION = libvirt.open("qemu:///system")

main_test_vm = VM(LIBVIRT_CONNECTION, MAIN_TEST_VM_NAME, MAIN_TEST_VM_DISK_SPACE)
second_test_vm = VM(LIBVIRT_CONNECTION, SECOND_TEST_VM_NAME, SECOND_TEST_VM_DISK_SPACE)
assert main_test_vm.vsock_cid != second_test_vm.vsock_cid

@given("I have an Ubuntu Desktop machine")
def step_impl(context: behave.runner.Context):
    force_new_vms = context.config.userdata.getbool("FORCE_NEW_VMS")

    if not force_new_vms and main_test_vm.has_snapshot(SNAPSHOT_BASE):
        main_test_vm.restore_snapshot(SNAPSHOT_BASE)
        return

    checkpoints.new_vm.restore_or_run(main_test_vm, force_new_vms)

    # Install authd and the authd-msentraid snap
    # TODO: This should not be done here
    main_test_vm.check_call(["sudo", "apt", "install", "-y", "authd"])
    main_test_vm.check_call(["sudo", "snap", "install", "authd-msentraid"])

    # Install the dogtail service
    # TODO: This should not be done here
    # Get the latest snap of the dogtail service from the dogtail service directory
    # snaps = glob.glob(os.path.join(DOGTAIL_SERVICE_DIR, "dogtail-service_*.snap"))
    # if not snaps:
    #     raise FileNotFoundError("No dogtail-service snap found in %s" % DOGTAIL_SERVICE_DIR)
    #
    # snap = max(snaps, key=os.path.getctime)
    # executil.check_call(["multipass", "transfer", snap, f"{MAIN_TEST_VM_NAME}:/tmp/"])
    # main_test_vm.check_call(["sudo", "snap", "install", "--devmode", "--dangerous", "/tmp/" + os.path.basename(snap)])
    # install_dogtail_service

    # Configure authd to use the MS Entra ID broker
    # TODO: This should not be done here
    src = "/snap/authd-msentraid/current/conf/authd/msentraid.conf"
    dest = "/etc/authd/brokers.d/"
    main_test_vm.check_call(["sudo", "install", "-D", "--target-directory", dest, src])

    # Configure the MS Entra ID broker to use the test OIDC app
    # TODO: This should not be done here
    issuer_id = context.config.userdata["msentraid_issuer_id"]
    client_id = context.config.userdata["msentraid_client_id"]
    broker_config_file = "/var/snap/authd-msentraid/current/broker.conf"
    main_test_vm.check_call([
                         "sudo", "sed", "-i", "-e", f"s/<ISSUER_ID>/{issuer_id}/", "-e",
                         f"s/<CLIENT_ID>/{client_id}/",
                         broker_config_file])

    # Create the snapshot
    main_test_vm.create_snapshot(SNAPSHOT_BASE, "Initial snapshot")


@given("I have a second machine with a web browser")
def step_impl(context: behave.runner.Context):
    force_new_vms = context.config.userdata.getbool("FORCE_NEW_VMS")
    checkpoints.second_vm_prepared.restore_or_run(
        second_test_vm, force_new_vms,
    )


@step("I installed the authd package")
def step_impl(context: behave.runner.Context):
    host_path = context.config.userdata["authd_package"]
    vm_path = f"/tmp/{os.path.basename(host_path)}"
    executil.check_call(["multipass", "copy-files", host_path, f"{MAIN_TEST_VM_NAME}:{vm_path}"])
    main_test_vm.check_call(["sudo", "dpkg", "-i", vm_path])


@step("I installed the authd-msentraid snap")
def step_impl(context: behave.runner.Context):
    host_path = context.config.userdata["authd_msentraid_snap"]
    vm_path = f"/tmp/{os.path.basename(host_path)}"
    executil.check_call(["multipass", "copy-files", host_path, f"{MAIN_TEST_VM_NAME}:{vm_path}"])
    main_test_vm.check_call(["sudo", "snap", "install", "--dangerous", vm_path])


@step("I configured authd to use the MS Entra ID broker")
def step_impl(context: behave.runner.Context):
    src = "/snap/authd-msentraid/current/conf/authd/msentraid.conf"
    dest = "/etc/authd/brokers.d/"
    main_test_vm.check_call(["sudo", "install", "-D", "--target-directory", dest, src])


@step("I configured the MS Entra ID broker to use the test OIDC app")
def step_impl(context: behave.runner.Context):
    issuer_id = context.config.userdata["msentraid_issuer_id"]
    client_id = context.config.userdata["msentraid_client_id"]
    broker_config_file = "/var/snap/authd-msentraid/current/broker.conf"
    main_test_vm.check_call([
        "sudo", "sed", "-i", "-e", f"s/<ISSUER_ID>/{issuer_id}/", "-e", f"s/<CLIENT_ID>/{client_id}/",
        broker_config_file],
    )


@step("I rebooted the system")
def step_impl(context: behave.runner.Context):
    main_test_vm.restart()


@step("I'm at the GDM login screen")
def step_impl(context: behave.runner.Context):
    # Check if we're at the GDM login screen
    node = main_test_vm.gnome_shell.find_child(name="Login Options", role_name="menu")
    logging.info("Login Options: %s", node)


@when('I enter the username of the test user')
def step_impl(context: behave.runner.Context):
    test_user = context.config.userdata["test_user_name"]

    # The username text entry doesn't have a label or description, but it's the only editable
    # text entry and the focused one.
    text_entry = main_test_vm.gnome_shell.find_child(role_name="text", editable=True, focused=True)
    text_entry.set_text(test_user)
    text_entry.activate()


@then("I am asked to select the broker")
def step_impl(context: behave.runner.Context):
    main_test_vm.gnome_shell.find_child(name="Select a broker", role_name="label")


@when('I select the "(?P<broker_name>.+)" broker')
def step_impl(context: behave.runner.Context, broker_name: str):
    # The push button is the parent of the label with the broker name
    label = main_test_vm.gnome_shell.find_child(name=broker_name, role_name="label")
    push_button = label.get_parent()
    # The push button doesn't expose any actions, so we need to make it grab focus and press Enter
    push_button.grab_focus()
    main_test_vm.screen.press("Enter")


@then('I see the message "(?P<message>.+)"')
def step_impl(context: behave.runner.Context, message: str):
    main_test_vm.gnome_shell.find_child(name=message, role_name="label")


@step('I see a QR code which encodes the URL "(?P<url>.+)"')
def step_impl(context: behave.runner.Context, url: str):
    with tempfile.NamedTemporaryFile(prefix="screenshot-", suffix=".png") as f:
        # Take the screenshot
        main_test_vm.screen.screenshot(f.name)
        # Parse the QR code using zbarimg
        output = executil.check_output(["zbarimg", "-q", "--raw", f.name])
        assert output.strip() == url


@step("I see a valid Microsoft Entra ID login code")
def step_impl(context: behave.runner.Context):
    label = main_test_vm.gnome_shell.find_child(name="Login code: ", role_name="label")
    # The login code is the next sibling of the "Login code: " label
    login_code_label = label.get_parent().get_children()[1]
    assert login_code_label.get_role_name() == "label", f"Expected a label, got {login_code_label.get_role_name()}"
    assert msentraid.is_valid_login_code(login_code_label.name), f"Invalid login code: {login_code_label.name}"


@when('I open "(?P<url>.+)" on the second machine')
def step_impl(context: behave.runner.Context, url: str):
    # Open the URL in the browser
    second_test_vm.check_call(["xdg-open", url])


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
