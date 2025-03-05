from typing import TYPE_CHECKING
from logging import getLogger

from checkpoint import Checkpoint

if TYPE_CHECKING:
    from vm import VM

logger = getLogger(__name__)

def _launch_new_vm(vm: "VM", force_new_snapshots=False) -> None:
    vm.ensure_is_purged()
    vm.launch()

    # Define the devices we need in the VM. That requires stopping the VM.
    vm.stop()
    vm.define_devices()
    vm.start()

    # Uninstall unattended-upgrades, because it can lock apt
    vm.check_call(["sudo", "apt", "remove", "-y", "unattended-upgrades"])

    # Add the authd PPA (to install gnome-shell and yaru-theme-gnome-shell from
    # the PPA)
    vm.check_call(["sudo", "add-apt-repository", "-y",
                   "ppa:ubuntu-enterprise-desktop/authd"])

    # Install the GNOME desktop
    vm.check_call(["sudo", "apt", "update"])
    vm.check_call(["sudo", "apt", "install", "-y", "gnome-session"])

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


new_vm = Checkpoint(
    "new-vm",
    "The VM is newly created",
    _launch_new_vm,
)

def _prepare_second_vm(vm: "VM", force_new_snapshots=False) -> None:
    new_vm.restore_or_run(vm, force_new_snapshots)

    ### Install packages needed in the second VM ###
    vm.check_call(["sudo", "apt", "update"])
    # The second machine needs a web browser
    vm.check_call(["sudo", "apt", "install", "-y", "firefox"])

    ### Create a user and log in ###
    username = "user"
    password = "test"

    # Create the user
    vm.check_call(["sudo", "useradd", "-m", username])
    vm.check_call(["sudo", "chpasswd"], input=f"{username}:{password}")

    # Restart the VM to be able to log in as the new user
    vm.restart()

    # Wait until we're at the GDM login screen
    vm.gnome_shell.find_child(name="Login Options", role_name="menu")

    # Enter the username. The username text entry doesn't have a label or description,
    # but it's the only editable text entry and the focused one.
    text_entry = vm.gnome_shell.find_child(
        role_name="text",
        editable=True,
        focused=True,
    )
    text_entry.set_text(username)
    text_entry.activate()

    # Enter the password
    password_entry = vm.gnome_shell.find_child(
        role_name="text",
        editable=True,
        focused=True,
    )
    password_entry.set_text(password)
    password_entry.activate()

second_vm_prepared = Checkpoint(
    "second-vm-prepared",
    "The second VM is prepared for testing",
    _prepare_second_vm,
)
