import time
import functools
from typing import Callable, Type, Tuple

from logging import getLogger

logger = getLogger(__name__)


def retryable(
    timeout_sec: float,
    interval_sec: float,
    retriable_exceptions: Tuple[Type[BaseException], ...],
    error_msg: str = "",
):
    def decorator(func):
        @functools.wraps(func)
        def wrapper(*args, **kwargs):
            start_time = time.monotonic()
            while time.monotonic() - start_time < timeout_sec:
                try:
                    return func(*args, **kwargs)
                except retriable_exceptions as e:
                    logger.debug("Retrying: %s", e)
                    time.sleep(interval_sec)
            raise TimeoutError(error_msg + f"(timeout: {timeout_sec} seconds)")
        return wrapper
    return decorator
