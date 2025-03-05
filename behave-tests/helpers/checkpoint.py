from typing import TYPE_CHECKING, Optional, Callable

if TYPE_CHECKING:
    from vm import VM

class Checkpoint:
    def __init__(self, name: str, description: str, run: "Callable[[VM, Optional[bool]], None]"):
        self.name = name
        self.description = description
        self.run = run

    def restore_or_run(self, vm: "VM", force_new_snapshots = False, *args, **kwargs) -> None:
        if  not force_new_snapshots and vm.has_snapshot(self.name):
            vm.restore_snapshot(self.name)
            return

        self.run(vm, force_new_snapshots, *args, **kwargs)
        vm.create_snapshot(self.name, self.description)
