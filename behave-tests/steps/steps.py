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

import executil
import msentraid
from accessible import Accessible, SearchError
from retry import retryable
from snapshot import snapshot
from util import retry, RetriableError
from vm import VM

logger = logging.getLogger(os.path.basename(__file__))

use_step_matcher("re")

USER_NAME = "ubuntu"
PASSWORD = "ubuntu"

MAIN_TEST_VM_NAME = "behave-tests-main"
MAIN_TEST_VM_DISK_SPACE = "5G"
MAIN_TEST_VM_MEMORY = "2G"
SECOND_TEST_VM_NAME = "behave-tests-second"
SECOND_TEST_VM_DISK_SPACE = "10G"
SECOND_TEST_VM_MEMORY = "2G"
SECOND_TEST_VM_USER = "user"
SECOND_TEST_VM_PASSWORD = "test"

LIBVIRT_CONNECTION = libvirt.open("qemu:///system")

main_test_vm = VM(LIBVIRT_CONNECTION,
                  MAIN_TEST_VM_NAME,
                  MAIN_TEST_VM_DISK_SPACE,
                  MAIN_TEST_VM_MEMORY)
second_test_vm = VM(LIBVIRT_CONNECTION,
                    SECOND_TEST_VM_NAME,
                    SECOND_TEST_VM_DISK_SPACE,
                    SECOND_TEST_VM_MEMORY)
assert main_test_vm.vsock_cid != second_test_vm.vsock_cid

login_code = None # type: str|None


@given("I have an Ubuntu Desktop machine set up to test authd and booted to the GDM login screen")
def step_impl(context: behave.runner.Context):
    force_new_vms = context.config.userdata.getbool("FORCE_NEW_VMS")
    # TODO: The snapshot should be invalidated if these change
    issuer_id = context.config.userdata["msentraid_issuer_id"]
    client_id = context.config.userdata["msentraid_client_id"]
    prepare_main_vm(main_test_vm, force_new_vms, issuer_id, client_id)


@snapshot("authd-installed-and-configured", "authd and the Entra ID broker are installed and configured")
def prepare_main_vm(vm: VM, force_new_snapshots: bool,
                    issuer_id: str, client_id: str) -> None:
    prepare_new_vm(vm, force_new_snapshots, no_reboot=True)

    # Install authd and the authd-msentraid snap
    vm.check_call(["sudo", "apt", "install", "-y", "authd"])
    vm.check_call(["sudo", "snap", "install", "authd-msentraid"])

    # Configure authd to use the MS Entra ID broker
    src = "/snap/authd-msentraid/current/conf/authd/msentraid.conf"
    dest = "/etc/authd/brokers.d/"
    vm.check_call(["sudo", "install", "-D", "--target-directory", dest, src])

    # Configure the MS Entra ID broker to use the test OIDC app
    broker_config_file = "/var/snap/authd-msentraid/current/broker.conf"
    vm.check_call([
        "sudo", "sed", "-i", "-e", f"s/<ISSUER_ID>/{issuer_id}/", "-e",
        f"s/<CLIENT_ID>/{client_id}/",
        broker_config_file,
    ])

    # Reboot the VM
    vm.restart()


@given("I have an Ubuntu Desktop machine")
def step_impl(context: behave.runner.Context):
    force_new_vms = context.config.userdata.getbool("FORCE_NEW_VMS")
    prepare_new_vm(main_test_vm, force_new_vms)

@given("I have an Ubuntu Desktop machine with the desktop-security-center installed")
def step_impl(context: behave.runner.Context):
    force_new_vms = context.config.userdata.getbool("FORCE_NEW_VMS")
    prepare_vm_for_apparmor_prompting(main_test_vm, force_new_vms)


@snapshot("apparmor-prompting", "The VM is prepared for AppArmor prompting")
def prepare_vm_for_apparmor_prompting(vm: VM, force_new_snapshots: bool) -> None:
    prepare_new_vm(vm, force_new_snapshots, no_reboot=True)

    # TODO: Remove once the snapshots were recreated
    vm.wait_until_running()

    # Install the desktop-security-center snap
    vm.check_call(["sudo", "snap", "install", "desktop-security-center"])

    # Restart the VM to apply the changes
    vm.restart()

@snapshot("new-vm", "The VM is newly created")
def prepare_new_vm(vm: VM, force_new_snapshots: bool, no_reboot: bool = False):
    vm.ensure_is_purged()
    vm.launch()

    # Define the devices we need in the VM. That requires stopping the VM.
    vm.stop()
    vm.define_devices()
    vm.start()

    # Set a root password
    vm.check_call(["sudo", "chpasswd"], input="root:root")

    # Set a password for the default user
    vm.check_call(["sudo", "chpasswd"], input=f"{USER_NAME}:{PASSWORD}")

    # Uninstall unattended-upgrades, because it can lock apt
    vm.check_call(["sudo", "apt", "remove", "-y", "unattended-upgrades"])

    # Add the authd PPA (to install gnome-shell and yaru-theme-gnome-shell from
    # the PPA)
    vm.check_call(["sudo", "add-apt-repository", "-y",
                   "ppa:ubuntu-enterprise-desktop/authd"])

    # Install the GNOME desktop
    vm.check_call(["sudo", "apt", "update"])
    vm.check_call(["sudo", "apt", "install", "-y", "ubuntu-session"])

    # Install socat
    vm.check_call(["sudo", "apt", "install", "-y", "socat"])

    # Enable anonymous authentication for the a11y bus, because we forward it
    # to the host and connect to it as the current user.
    logger.debug("Enabling anonymous authentication to the a11y bus")
    old_config = "<auth>EXTERNAL</auth>"
    new_config = "<auth>EXTERNAL</auth>\\n  " \
                 "<auth>ANONYMOUS</auth>\\n  " \
                 "<allow_anonymous/>\\n  "
    vm.check_call(["sudo", "sed", "-i", f"s|{old_config}|{new_config}|",
                   "/usr/share/defaults/at-spi2/accessibility.conf"])

    # Set GNOME_ACCESSIBILITY=1 in /etc/environment, which is needed for
    # Firefox (and maybe other apps) to expose itself on the a11y bus
    vm.check_call(["sudo", "sh", "-c",
                   "echo GNOME_ACCESSIBILITY=1 > /etc/environment.d/90-gnome-a11y.conf"])

    if not no_reboot:
        # Restart the VM to apply the changes
        vm.restart()


@given("I have a second machine with a web browser")
def step_impl(context: behave.runner.Context):
    force_new_vms = context.config.userdata.getbool("FORCE_NEW_VMS")
    prepare_second_vm(second_test_vm, force_new_vms)


@snapshot("second-vm-prepared", "The second VM is prepared for testing")
def prepare_second_vm(vm: VM, force_new_snapshots: bool) -> None:
    prepare_new_vm(vm, force_new_snapshots, no_reboot=True)

    # TODO: Delete this, it's already done in prepare_new_vm
    # Set a root password
    vm.check_call(["sudo", "chpasswd"], input="root:root")

    # The second machine needs a web browser
    install_firefox(vm, force_new_snapshots)

    # TODO: Remove this, only do it in _launch_new_vm
    # Set GNOME_ACCESSIBILITY=1 in /etc/environment, which is needed for
    # Firefox (and maybe other apps) to expose itself on the a11y bus
    # vm.check_call(["sudo", "sh", "-c",
    #                "echo GNOME_ACCESSIBILITY=1 > /etc/environment.d/90-gnome-a11y.conf"])

    ### Set a password for the default user and log in ###
    username = SECOND_TEST_VM_USER
    password = SECOND_TEST_VM_PASSWORD

    # Set a password for the default user
    vm.check_call(["sudo", "chpasswd"], input=f"{username}:{password}")

    # Enable accessibility for the user
    vm.check_call(["sudo", "su", username, "-c",
                   "gsettings set org.gnome.desktop.interface toolkit-accessibility true"])

    # Restart the VM to be able to log in as the new user
    vm.restart()

    # Wait until we're at the GDM login screen
    vm.gnome_shell.find_child(name="Login Options", role_name="menu")

    user_button = vm.gnome_shell.find_child(role_name="push button",
                                            label=username)
    user_button.grab_focus()
    vm.screen.press("Enter")

    # Enter the password
    password_entry = vm.gnome_shell.find_child(
        role_name="password text",
        editable=True,
    )
    password_entry.set_text(password)
    password_entry.activate()

@snapshot("firefox-installed", "Firefox installed")
def install_firefox(vm: VM, force_new_snapshots: bool) -> None:
    vm.check_call(["sudo", "apt", "update"])
    vm.check_call(["sudo", "apt", "install", "-y", "firefox"])

@given("I logged in")
def step_impl(context: behave.runner.Context):
    # Wait until we're at the GDM login screen
    main_test_vm.gnome_shell.find_child(name="Login Options", role_name="menu")

    # Set a password for the default user
    # TODO: Remove once the snapshots were recreated
    main_test_vm.check_call(["sudo", "chpasswd"], input=f"{USER_NAME}:{PASSWORD}")

    # The username text entry doesn't have a label or description, but it's the only editable
    # text entry.
    text_entry = main_test_vm.gnome_shell.find_child(role_name="text",
                                                     editable=True)
    text_entry.set_text(USER_NAME)
    text_entry.activate()

    # Enter the password
    password_entry = main_test_vm.gnome_shell.find_child(
        role_name="password text",
        editable=True,
    )
    password_entry.set_text(PASSWORD)
    password_entry.activate()

    # Use the a11y bus of the logged-in user from now on
    main_test_vm.a11y_bus_user = USER_NAME

    # Wait for the desktop to load
    main_test_vm.gnome_shell.find_child(name="Activities",
                                        retry=True,
                                        retry_timeout=30,
                                        retry_interval=1)


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
    main_test_vm.gnome_shell.find_child(
        name="Login Options", role_name="menu",
        retry=True, retry_timeout=30, retry_interval=1,
    )


@when('I enter the username of the test user')
def step_impl(context: behave.runner.Context):
    test_user = context.config.userdata["test_user_name"]

    # The username text entry doesn't have a label or description, but it's the only editable
    # text entry.
    text_entry = main_test_vm.gnome_shell.find_child(role_name="text", editable=True)
    text_entry.set_text(test_user)
    text_entry.activate()


@then("I am asked to select the broker")
def step_impl(context: behave.runner.Context):
    main_test_vm.gnome_shell.find_child(name="Select the broker", role_name="label",
                                        retry=True, retry_timeout=10)


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
    main_test_vm.gnome_shell.find_child(name=message, role_name="label", retry=True)


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
    # The login code is the next sibling of the "Login code: " label
    sibling = main_test_vm.gnome_shell.find_child(name="Login code: ", role_name="label")
    login_code_label = sibling.get_parent().get_children()[1]
    assert login_code_label.get_role_name() == "label", f"Expected a label, got {login_code_label.get_role_name()}"

    # Save the login code for later
    global login_code
    login_code = login_code_label.name
    assert msentraid.is_valid_login_code(login_code), f"Invalid login code: {login_code}"


@when('I open "(?P<url>.+)" on the second machine and log in')
def step_impl(context: behave.runner.Context, url: str):
    # Use the a11y bus of the logged-in user
    second_test_vm.a11y_bus_user = SECOND_TEST_VM_USER

    # Launch Firefox
    search_entry = second_test_vm.gnome_shell.find_child(editable=True, role_name="text")
    search_entry.set_text("firefox")

    push_button = second_test_vm.gnome_shell.find_child(role_name="push button", label="Firefox",
                                                        retry=True)
    push_button.grab_focus()
    second_test_vm.screen.press("Enter")

    # Wait for Firefox to start
    @retryable(30, 1, (SearchError,), "Firefox is not running")
    def find_firefox():
        return second_test_vm.application("Firefox")
    firefox = find_firefox()

    address_bar = firefox.find_child("Search or enter address", role_name="entry", editable=True)
    # XXX: set_text doesn't work because of https://bugzilla.mozilla.org/show_bug.cgi?id=1861026
    address_bar.grab_focus()
    second_test_vm.screen.paste(url)

    # import ipdb
    # ipdb.set_trace()
    #
    # address_bar.activate()


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


@step('I enter the login code "user_code"')
def step_impl(context):
    """
    :type context: behave.runner.Context
    """
    raise NotImplementedError(u'STEP: And I enter the login code "user_code"')


@step(
    'I log in with the username "demo@uaadtest\.onmicrosoft\.com" and password "password"')
def step_impl(context):
    """
    :type context: behave.runner.Context
    """
    raise NotImplementedError(
        u'STEP: And I log in with the username "demo@uaadtest.onmicrosoft.com" and password "password"')


@then('I am asked if I am trying to sign in to "Azure OIDC Poc"')
def step_impl(context):
    """
    :type context: behave.runner.Context
    """
    raise NotImplementedError(
        u'STEP: Then I am asked if I am trying to sign in to "Azure OIDC Poc"')


@when('I click "Continue"')
def step_impl(context):
    """
    :type context: behave.runner.Context
    """
    raise NotImplementedError(u'STEP: When I click "Continue"')


@when('I launch "(?P<app_name>.+)"')
def step_impl(context, app_name: str):
    search_entry = main_test_vm.gnome_shell.find_child(editable=True,
                                                       role_name="text",
                                                       retry=True)
    search_entry.set_text(app_name)

    push_button = second_test_vm.gnome_shell.find_child(role_name="push button", label=app_name,
                                                        retry=True)
    push_button.grab_focus()
    second_test_vm.screen.press("Enter")

    # Wait for the app to start
    def check_app_running():
        try:
            return second_test_vm.application(app_name)
        except SearchError:
            raise RetriableError(f"{app_name} not running yet")
    retry(check_app_running, 30, 1)

@when('I launch the Security Center')
def step_impl(context):
    search_entry = main_test_vm.gnome_shell.find_child(editable=True,
                                                       role_name="text",
                                                       retry=True)
    search_entry.set_text("Security Center")

    push_button = second_test_vm.gnome_shell.find_child(role_name="push button",
                                                        label="Settings",
                                                        retry=True)
    push_button.grab_focus()
    second_test_vm.screen.press("Enter")

    app_name = "Security Center"

    # Wait for the app to start
    def check_app_running():
        try:
            return second_test_vm.application(app_name)
        except SearchError:
            raise RetriableError(f"{app_name} not running yet")
    retry(check_app_running, 30, 1)
