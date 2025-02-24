import logging
import subprocess
import sys

logger = logging.getLogger(__name__)

def _run(cmd: list, *args, **kwargs) -> subprocess.CompletedProcess:
    """Run a command and print it's stderr continuously but also return
    stderr in the return CompletedProcess and any raised CalledProcessError."""
    cmd = [str(s) for s in cmd]
    # This method will be called from executil.run() (or check_call, or check_output),
    # and we want to attribute the log message to its caller
    logger.debug(f"Executing command `{' '.join(cmd)}`", stacklevel=3)

    hide_stderr = kwargs.get("stderr") == subprocess.DEVNULL

    kwargs["stderr"] = subprocess.PIPE
    kwargs["text"] = True
    try:
        p = subprocess.run(cmd, *args, **kwargs)  # noqa: PLW1510
    except subprocess.CalledProcessError as e:
        if not hide_stderr:
            print(e.stderr, file=sys.stderr)
        raise
    else:
        if not hide_stderr:
            print(p.stderr, file=sys.stderr)
        return p
    finally:
        logger.debug(f"Done executing command `{' '.join(cmd)}`", stacklevel=3)


def run(cmd: list, *args, **kwargs) -> subprocess.CompletedProcess:
    return _run(cmd, *args, **kwargs)


def check_call(cmd: list, *args, **kwargs):
    return _run(cmd, *args, **kwargs, check=True)


def check_output(cmd: list, *args, **kwargs) -> str:
    p = _run(cmd, *args, **kwargs, check=True, stdout=subprocess.PIPE)
    return p.stdout