from screen import Screen
import executil

WAIT_FOR_VM_STOPPED_TIMEOUT = 30
WAIT_FOR_VM_RUNNING_TIMEOUT = 30

class VM:
    def __init__(self, libvirt_connection: libvirt.virConnect, vm_name: str):
        self.libvirt_connection = libvirt_connection
        self.vm_name = vm_name
        self.domain = libvirt_connection.lookupByName(vm_name)
        self.screen = Screen(self.domain)

    def start(self):
        logging.debug("Starting VM '%s'", self.vm_name)
        executil.check_call(["multipass", "start", self.vm_name])
        self.wait_until_running()

    def stop(self):
        logging.debug("Stopping VM '%s'", self.vm_name)
        executil.check_call(["multipass", "stop", self.vm_name])
        self.wait_until_stopped()

    def restart(self):
        self.stop()
        self.start()

    def check_call(self, command: [str]):
        executil.check_call(["multipass", "exec", self.vm_name, "--"] + command)

    def get_ip(self):
        pass

    def get_vsock(self):
        pass

    def get_vsock_port(self):
        pass

    def define_devices(self):
        # Remove all video devices
        root = ET.fromstring(self.domain.XMLDesc())
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

        # Attach a vsock device
        #  <vsock model='virtio'>
        #    <cid auto='no' address='3'/>
        #  </vsock>
        logging.debug("Attaching a vsock device")
        vsock = ET.Element("vsock", model="virtio")
        ET.SubElement(vsock, "cid", auto="no", address="3")
        root.find("devices").append(vsock)

        # Re-define the domain
        domain = conn.defineXMLFlags(ET.tostring(root).decode(), libvirt.VIR_DOMAIN_DEFINE_VALIDATE)

    def wait_until_stopped(self):
        logging.debug("Waiting for VM '%s' to stop", name)
        start_time = time.monotonic()
        while time.monotonic() - start_time < WAIT_FOR_VM_STOPPED_TIMEOUT:
            try:
                vm_info_json = executil.check_output(["multipass", "info", name, "--format", "json"])
            except subprocess.CalledProcessError as e:
                if e.returncode == 2:
                    # The VM does not exist
                    time.sleep(1)
                    continue
                else:
                    # Unexpected error
                    raise

            vm_info = json.loads(vm_info_json)
            if vm_info["info"][name]["state"] == "Stopped":
                return

            time.sleep(1)

        raise TimeoutError(f"VM '{self.vm_name}' did not stop within the timeout ({WAIT_FOR_VM_STOPPED_TIMEOUT} seconds)")

    def wait_until_running(name: str):
        start_time = time.monotonic()
        while time.monotonic() - start_time < WAIT_FOR_VM_RUNNING_TIMEOUT:
            try:
                vm_info_json = executil.check_output(["multipass", "info", name, "--format", "json"])
            except subprocess.CalledProcessError as e:
                if e.returncode == 2:
                    # The VM does not exist
                    time.sleep(1)
                    continue
                else:
                    # Unexpected error
                    raise

            vm_info = json.loads(vm_info_json)
            if vm_info["info"][name]["state"] == "Running":
                return

            time.sleep(1)

        raise TimeoutError(f"VM '{self.vm_name}' did not start within the timeout ({WAIT_FOR_VM_RUNNING_TIMEOUT} seconds)")
