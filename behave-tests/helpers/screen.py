import logging

import libvirt

from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from vm import VM

keymap = {
    "a": 0x1e, "b": 0x30, "c": 0x2e, "d": 0x20, "e": 0x12, "f": 0x21, "g": 0x22, "h": 0x23, "i": 0x17, "j": 0x24,
    "k": 0x25, "l": 0x26, "m": 0x32, "n": 0x31, "o": 0x18, "p": 0x19, "q": 0x10, "r": 0x13, "s": 0x1f, "t": 0x14,
    "u": 0x16, "v": 0x2f, "w": 0x11, "x": 0x2d, "y": 0x15, "z": 0x2c,

    "1": 0x02, "2": 0x03, "3": 0x04, "4": 0x05, "5": 0x06, "6": 0x07, "7": 0x08, "8": 0x09, "9": 0x0a, "0": 0x0b,

    "enter": 0x1c, "esc": 0x01, "backspace": 0x0e, "tab": 0x0f, "space": 0x39, "minus": 0x0c, "equal": 0x0d,
    "leftbracket": 0x1a, "rightbracket": 0x1b, "backslash": 0x2b, "semicolon": 0x27, "apostrophe": 0x28,
    "grave": 0x29, "comma": 0x33, "dot": 0x34, "slash": 0x35,

    "f1": 0x3b, "f2": 0x3c, "f3": 0x3d, "f4": 0x3e, "f5": 0x3f, "f6": 0x40, "f7": 0x41, "f8": 0x42, "f9": 0x43,
    "f10": 0x44, "f11": 0x57, "f12": 0x58,

    "insert": 0xd2, "delete": 0xd3, "home": 0xc7, "end": 0xcf, "pageup": 0xc9, "pagedown": 0xd1,

    "right": 0xcd, "left": 0xcb, "down": 0xd0, "up": 0xc8,

    "numlock": 0x45, "kp_slash": 0xb5, "kp_multiply": 0x37, "kp_minus": 0x4a, "kp_plus": 0x4e, "kp_enter": 0x9c,

    "kp_1": 0x4f, "kp_2": 0x50, "kp_3": 0x51, "kp_4": 0x4b, "kp_5": 0x4c, "kp_6": 0x4d, "kp_7": 0x47, "kp_8": 0x48,

    "kp_9": 0x49, "kp_0": 0x52, "kp_dot": 0x53,

    "lctrl": 0x1d, "rctrl": 0x9d, "lshift": 0x2a, "rshift": 0x36, "lalt": 0x38, "ralt": 0xb8, "lmeta": 0xdb,

    "rmeta": 0xdc, "compose": 0xe0, "power": 0xde, "sleep": 0xdf, "wake": 0xe3, "menu": 0xdd,
}

def keycode(key: str) -> int:
    # Convert to lowercase because the keymap is lowercase
    return keymap[key.lower()]


class Screen:
    def __init__(self, vm: "VM"):
        self.vm = vm

    def press(self, *keys: str) -> None:
        keycodes = [keycode(key) for key in keys]
        self.vm.domain.sendKey(
            codeset=libvirt.VIR_KEYCODE_SET_LINUX,
            holdtime=40,
            keycodes=keycodes,
            nkeycodes=len(keycodes),
            flags=0,
        )

    def screenshot(self, filename: str, screen_id=0) -> None:
        with open(filename, "wb") as f:
            stream = self.vm.libvirt_connection.newStream()
            mime_type = self.vm.domain.screenshot(stream, screen_id)
            while True:
                data = stream.recv(1024 * 64)
                if data is None or len(data) == 0:
                    break
                if isinstance(data, int):
                    raise Exception(f"Error while taking screenshot: {data}")
                f.write(data)
            stream.finish()

    def paste(self, text: str):
        self.vm.set_clipboard(text)
        self.press("lctrl", "v")
