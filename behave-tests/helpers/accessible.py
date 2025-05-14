import logging
from typing import Optional

import util

import gi

from retry import retryable

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
    ) # type: Gio.DBusProxy

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
        )  # type: Gio.DBusProxy | None

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

        if "org.a11y.atspi.Text" in self.interfaces:
            self._text_proxy = Gio.DBusProxy.new_sync(
                connection=connection,
                flags=Gio.DBusProxyFlags.NONE,
                info=None,
                name=bus_name,
                object_path=path,
                interface_name="org.a11y.atspi.Text",
                cancellable=None
            ) # type: Gio.DBusProxy | None
        else:
            self._text_proxy = None

        if "org.a11y.atspi.EditableText" in self.interfaces:
            self._editable_text_proxy = Gio.DBusProxy.new_sync(
                connection=connection,
                flags=Gio.DBusProxyFlags.NONE,
                info=None,
                name=bus_name,
                object_path=path,
                interface_name="org.a11y.atspi.EditableText",
                cancellable=None
            ) # type: Gio.DBusProxy | None
        else:
            self._editable_text_proxy = None

        if "org.a11y.atspi.Component" in self.interfaces:
            self._component_proxy = Gio.DBusProxy.new_sync(
                connection=connection,
                flags=Gio.DBusProxyFlags.NONE,
                info=None,
                name=bus_name,
                object_path=path,
                interface_name="org.a11y.atspi.Component",
                cancellable=None
            ) # type: Gio.DBusProxy | None
        else:
            self._component_proxy = None

        _name = self._proxy.get_cached_property("Name")
        self.name = _name.unpack() if _name is not None else None
        _description = self._proxy.get_cached_property("Description")
        self.description = _description.unpack() if _description is not None else None
        _help_text = self._proxy.get_cached_property("HelpText")
        self.help_text = _help_text.unpack() if _help_text is not None else None

    def __str__(self):
        return f"(name={self.name!r}, role_name={self.get_role_name()!r}, description={self.description!r}, labels={self.get_labels()!r}, role_name={self.get_role_name()!r}, states={self.get_states()!r}, bus_name={self.bus_name!r}, path={self.path!r})"

    def __repr__(self):
        return str(self)

    def get_children(self) -> list["Accessible"]:
        children = list()
        get_children_resp = self._proxy.call_sync("GetChildren", None, Gio.DBusCallFlags.NONE, -1, None).unpack()[0]
        for _tuple in get_children_resp:
            bus_name, path = _tuple
            children.append(Accessible(self._connection, bus_name, path))
        return children

    def get_role_name(self) -> str:
        return self._proxy.call_sync("GetRoleName", None, Gio.DBusCallFlags.NONE, -1, None).unpack()[0]

    def get_labels(self) -> list["Accessible"]:
        relation_set = self._proxy.call_sync(
            method_name="GetRelationSet",
            parameters=None,
            flags=Gio.DBusCallFlags.NONE,
            timeout_msec=-1,
            cancellable=None
        ).unpack()[0]

        for (relation_type, targets) in relation_set:
            if relation_type == Atspi.RelationType.LABELLED_BY:
                return [Accessible(self._connection, bus_name, path) for bus_name, path in targets]
        return []

    def get_actions(self) -> list["Action"]:
        if self._actions_proxy is None:
            return []

        num_actions = self._actions_proxy.get_cached_property("NActions").unpack()
        return [Action(self._connection, self.bus_name, self.path, i) for i in range(num_actions)]

    def get_states(self) -> list[str]:
        state = self._proxy.call_sync(
            method_name="GetState",
            parameters=None,
            flags=Gio.DBusCallFlags.NONE,
            timeout_msec=-1,
            cancellable=None
        ).unpack()[0]
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

    def get_focused(self) -> bool:
        return "focused" in self.get_states()

    def get_parent(self) -> Optional["Accessible"]:
        bus_name, path = self._proxy.get_cached_property("Parent").unpack()
        if path is None:
            return None
        return Accessible(self._connection, bus_name, path)

    def find_child(
            self,
            name: str = None,
            role_name: str = None,
            description: str = None,
            label: str = None,
            editable: bool = None,
            focused: bool = None,
            retry: bool = False,
            retry_timeout: float = 5,
            retry_interval: float = 0.2
    ) -> "Accessible":
        """
        Recursively searches for a child matching the given criteria.
        Returns the first match found or None if no match is found.
        """
        query_description = f"(name={name!r}, description={description!r}, label={label!r}, role_name={role_name!r}, editable={editable!r}, focused={focused!r})"

        def find_child():
            result = self._find_child_recursive(name, role_name, description, label, editable, focused)
            if not result:
                raise SearchError(f"Could not find child with {query_description}")
            return result

        if not retry:
            return find_child()

        @retryable(timeout_sec=retry_timeout, interval_sec=retry_interval, retriable_exceptions=(SearchError,),
                   error_msg=f"Could not find child with {query_description}")
        def find_child_with_retry():
            return find_child()

        return find_child_with_retry()

    def _find_child_recursive(
            self,
            name: str = None,
            role_name: str = None,
            description: str = None,
            label: str = None,
            editable: bool = None,
            focused: bool = None,
    ) -> Optional["Accessible"]:
        """
        Recursively searches for a child matching the given criteria.
        Returns the first match found or None if no match is found.
        """
        query_description = f"(name={name!r}, description={description!r}, label={label!r}, role_name={role_name!r}, editable={editable!r}, focused={focused!r})"

        if editable is not None or focused is not None:
            states = self.get_states()
        else:
            states = []

        # logging.debug(f"XXX: Checking if query {query_description} matches {self}")

        if ((name is None or self.name == name) and
            (role_name is None or self.get_role_name() == role_name) and
            (description is None or self.description == description) and
            (label is None or any(label == l.name for l in self.get_labels())) and
            (editable is None or "editable" in states) and
            (focused is None or "focused" in states)):
            logging.debug(f"XXX: Found child with {query_description}: {self}")
            return self

        for child in self.get_children():
            result = child._find_child_recursive(name, role_name, description, label, editable, focused)
            if result:
                return result

        return None


    def grab_focus(self):
        success = self._component_proxy.call_sync(
            method_name="GrabFocus",
            parameters=None,
            flags=Gio.DBusCallFlags.NONE,
            timeout_msec=-1,
            cancellable=None
        )
        if not success:
            raise Exception(f"Could not grab focus for {self}")


    def click(self):
        self.do_action_named("click")

    def activate(self):
        self.do_action_named("activate")

    def do_action_named(self, name: str):
        actions = self.get_actions()
        for action in actions:
            if action.get_name() == name:
                action.do()
                return
        raise Exception(f"Could not find action '{name}' for {self}")

    def get_character_count(self) -> int:
        return self._text_proxy.call_sync(
            method_name='org.freedesktop.DBus.Properties.Get',
            parameters=GLib.Variant("(ss)", ("org.a11y.atspi.Text", "CharacterCount")),
            flags=Gio.DBusCallFlags.NONE,
            timeout_msec=-1,
            cancellable=None
        ).unpack()[0]

    def set_text(self, text: str):
        @retryable(timeout_sec=5, interval_sec=0.2, retriable_exceptions=(util.RetriableError,),
                   error_msg=f"Failed to set text '{text}' for {self}")
        def set_text_with_retry():
            success = self._editable_text_proxy.call_sync(
                method_name="SetTextContents",
                parameters=GLib.Variant("(s)", (text,)),
                flags=Gio.DBusCallFlags.NONE,
                timeout_msec=-1,
                cancellable=None
            )
            if not success:
                raise Exception(f"Could not set text '{text}' for {self}")

            # We can't use `if self.get_text != text` here, because that fails
            # if the entry is a password entry, for which get_text returns '●●●●'.
            # After taking a quick look, I couldn't find a way to figure out via
            # the a11y API if an entry is a password entry.
            if self.get_character_count() != len(text):
                raise util.RetriableError("Failed to set text")

        set_text_with_retry()

    def insert_text(self, text: str, start_position: int = 0):
        success = self._editable_text_proxy.call_sync(
            method_name="InsertText",
            parameters=GLib.Variant("(isi)", (start_position, text, 0)),
            flags=Gio.DBusCallFlags.NONE,
            timeout_msec=-1,
            cancellable=None
        )
        if not success:
            raise Exception(f"Could not insert text '{text}' for {self}")

    def delete_text(self, start_position: int = 0, end_position: int = None):
        if end_position is None:
            end_position = self.get_character_count()

        success = self._editable_text_proxy.call_sync(
            method_name="DeleteText",
            parameters=GLib.Variant("(ii)", (start_position, end_position)),
            flags=Gio.DBusCallFlags.NONE,
            timeout_msec=-1,
            cancellable=None
        )
        if not success:
            raise Exception(f"Could not delete text for {self}")

    def get_text(self, start_offset: int = None, end_offset: int = None) -> str:
        if start_offset is None:
            start_offset = 0
        if end_offset is None:
            end_offset = self.get_character_count()

        logging.debug(f"XXX: Getting text from {start_offset} to {end_offset} for {self}")

        return self._text_proxy.call_sync(
            method_name="GetText",
            parameters=GLib.Variant("(ii)", (start_offset, end_offset)),
            flags=Gio.DBusCallFlags.NONE,
            timeout_msec=-1,
            cancellable=None
        ).unpack()[0]


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

    def __str__(self):
        return f"(name={self.get_name()}, bus_name={self.bus_name}, path={self.path}, index={self.index})"

    def get_name(self) -> str:
        return self._proxy.call_sync(
            "GetName",
            GLib.Variant("(i)", (self.index,)),
            Gio.DBusCallFlags.NONE,
            -1,
            None
        ).unpack()[0]

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


def application_root(connection: Gio.DBusConnection, name: str) -> Root:
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
    raise SearchError(f"Could not find application root with name '{name}'")

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


class SearchError(Exception):
    pass