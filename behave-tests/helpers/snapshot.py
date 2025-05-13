import functools
import os

from logging import getLogger
logger = getLogger(os.path.basename(__file__))

from typing import TYPE_CHECKING
if TYPE_CHECKING:
    from vm import VM


def snapshot(snapshot_name: str, snapshot_description: str):
    def decorator(func):
        @functools.wraps(func)
        def wrapper(vm: "VM", force_new_snapshots: bool, *args, **kwargs):
            if not force_new_snapshots and vm.has_snapshot(snapshot_name):
                vm.restore_snapshot(snapshot_name)
                return
            result = func(vm, force_new_snapshots, *args, **kwargs)
            vm.create_snapshot(snapshot_name, snapshot_description)
            return result
        return wrapper
    return decorator
