import logging

import gi
gi.require_version("Atspi", "2.0")
from gi.repository import Gio, Atspi, GObject, GLib


def bus_names(connection: Gio.DBusConnection):
    """Retrieve a list of unique D-Bus names from the accessibility bus."""
    proxy = Gio.DBusProxy.new_sync(
        connection=connection,
        flags=Gio.DBusProxyFlags.NONE,
        info=None,
        name="org.freedesktop.DBus",
        object_path="/org/freedesktop/DBus",
        interface_name="org.freedesktop.DBus",
        cancellable=None
    )

    names = proxy.call_sync(
        "ListNames", None, Gio.DBusCallFlags.NONE, -1, None
    ).unpack()[0]

    return sorted(set(names))


class Accessible:
    def __init__(self, connection: Gio.DBusConnection, bus_name: str, path: str):
        self._connection = connection
        self.bus_name = bus_name
        self.path = path
        # logging.debug(f"Connecting to bus '{bus_name}', path '{path}', interface 'org.a11y.atspi.Accessible'")
        self._proxy = Gio.DBusProxy.new_sync(
            connection=connection,
            flags=Gio.DBusProxyFlags.NONE,
            info=None,
            name=bus_name,
            object_path=path,
            interface_name="org.a11y.atspi.Accessible",
            cancellable=None
        )  # type: Gio.DBusProxy

        try:
            self.interfaces = self._proxy.call_sync(
                method_name="GetInterfaces",
                parameters=None,
                flags=Gio.DBusCallFlags.NONE,
                timeout_msec=-1,
                cancellable=None
            ).unpack()[0] # type: list[str]
        except GLib.Error as e:
            if "org.freedesktop.DBus.Error.UnknownMethod" in e.message:
                self.interfaces = []
            else:
                raise

        if "org.a11y.atspi.Action" in self.interfaces:
            self._actions_proxy = Gio.DBusProxy.new_sync(
                connection=connection,
                flags=Gio.DBusProxyFlags.NONE,
                info=None,
                name=bus_name,
                object_path=path,
                interface_name="org.a11y.atspi.Action",
                cancellable=None
            )
        else:
            self._actions_proxy = None

        _name = self._proxy.get_cached_property("Name")
        self.name = _name.unpack() if _name is not None else None
        _description = self._proxy.get_cached_property("Description")
        self.description = _description.unpack() if _description is not None else None

    def get_children(self) -> list["Accessible"]:
        children = list()
        get_children_resp = self._proxy.call_sync("GetChildren", None, Gio.DBusCallFlags.NONE, -1, None).unpack()[0]
        for _tuple in get_children_resp:
            bus_name, path = _tuple
            children.append(Accessible(self._connection, bus_name, path))
        return children

    def get_role_name(self) -> str:
        return self._proxy.call_sync("GetRoleName", None, Gio.DBusCallFlags.NONE, -1, None).unpack()[0]

    def get_description(self) -> str:
        return self._proxy.get_cached_property("Description").unpack()

    def get_help_text(self) -> str:
        return self._proxy.get_cached_property("HelpText").unpack()

    def get_actions(self) -> list["Action"]:
        if self._actions_proxy is None:
            return []

        num_actions = self._actions_proxy.get_cached_property("NActions").unpack()
        return [Action(self._connection, self.bus_name, self.path, i) for i in range(num_actions)]

    def get_states(self) -> list[str]:
        # Get states
        state = self._proxy.call_sync("GetState", None, Gio.DBusCallFlags.NONE, -1, None).unpack()[0]
        # GetState returns an array but it seems like only the first element is ever non-zero
        state = state[0]

        # Convert the Atspi.StateType (based on GObject.GEnum) to a list of state names.
        state_names = []
        for state_type in Atspi.StateType.__enum_values__:
            if not state & (1 << state_type):
                continue

            s = GObject.enum_to_string(Atspi.StateType, Atspi.StateType(state_type))
            s = s.removeprefix("ATSPI_STATE_").lower()
            state_names.append(s)

        return state_names

    def find_child(self, name: str = None, role_name: str = None, description: str = None) -> "Accessible":
        for child in self.get_children():
            if name is not None and child.name != name:
                continue
            if role_name is not None and child.get_role_name() != role_name:
                continue
            if description is not None and child.get_description() != description:
                continue
            return child
        return None


class Action:
    def __init__(self, connection: Gio.DBusConnection, bus_name: str, path: str, index: int):
        self._connection = connection
        self.bus_name = bus_name
        self.path = path
        self.index = index
        # logging.debug(f"Connecting to bus '{bus_name}', path '{path}', interface 'org.a11y.atspi.Action'")
        self._proxy = Gio.DBusProxy.new_sync(
            connection=connection,
            flags=Gio.DBusProxyFlags.NONE,
            info=None,
            name=bus_name,
            object_path=path,
            interface_name="org.a11y.atspi.Action",
            cancellable=None
        )

    def get_name(self) -> str:
        return self._proxy.call_sync(
            "GetName",
            GLib.Variant("(i)", (self.index,)),
            Gio.DBusCallFlags.NONE,
            -1,
            None
        ).unpack()[0].upper()

    def do(self):
        self._proxy.call_sync(
            "DoAction",
            GLib.Variant("(i)", (self.index,)),
            Gio.DBusCallFlags.NONE,
            -1,
            None
        )

class Root(Accessible):
    def __init__(self, connection: Gio.DBusConnection, bus_name: str):
        super().__init__(connection, bus_name, "/org/a11y/atspi/accessible/root")


def application_root(connection: Gio.DBusConnection, name: str) -> Root|None:
    for bus_name in bus_names(connection):
        if not bus_name.startswith(":"):
            # Ignore well-known names, they are not applications
            continue

        try:
            if Root(connection, bus_name).name == name:
                return Root(connection, bus_name)
        except GLib.Error as e:
            if "GDBus.Error:org.freedesktop.DBus.Error.UnknownInterface" in e.message:
                # The bus does not implement the org.a11y.atspi.Accessible interface,
                # so it's not an application root
                continue
            raise

def application_roots(connection: Gio.DBusConnection) -> list[Root]:
    roots = list()
    for bus_name in bus_names(connection):
        if not bus_name.startswith(":"):
            # Ignore well-known names, they are not applications
            continue

        try:
            roots.append(Root(connection, bus_name))
        except GLib.Error as e:
            if "GDBus.Error:org.freedesktop.DBus.Error.UnknownInterface" in e.message:
                # The bus does not implement the org.a11y.atspi.Accessible interface,
                # so it's not an application root
                continue
            raise
    return roots
