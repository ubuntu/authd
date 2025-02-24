from logging import getLogger
import time

logger = getLogger(__name__)

class RetriableError(Exception):
    pass

def retry(func, timeout_sec: float, interval_sec: float, error_msg: str = ""):
    start_time = time.monotonic()
    while time.monotonic() - start_time < timeout_sec:
        try:
            return func()
        except RetriableError as e:
            logger.debug("Retrying: %s", e)
            time.sleep(interval_sec)

    raise TimeoutError(error_msg + f"(timeout: {timeout_sec} seconds)")
