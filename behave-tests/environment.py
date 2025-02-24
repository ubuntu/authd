import logging

from behave.model import Feature

from behave.model_core import Status

logging.basicConfig(level=logging.DEBUG,
                    format='%(asctime)s:%(levelname)s: %(message)s')

# This is not actually the class that behave passes to the functions
# below, but pretending that it is provides code completion
class EnvironmentContext:
    # Behave internal
    feature: Feature

# Enable Debug-on-Error support as described in
# https://behave.readthedocs.io/en/stable/tutorial.html#debug-on-error-in-case-of-step-failures

BEHAVE_DEBUG_ON_ERROR = False


def setup_debug_on_error(userdata):
    global BEHAVE_DEBUG_ON_ERROR  # noqa: PLW0603
    BEHAVE_DEBUG_ON_ERROR = userdata.getbool("BEHAVE_DEBUG_ON_ERROR")


def before_all(context):
    setup_debug_on_error(context.config.userdata)

    # Check if multipass is configured to use libvirt as the backend, so that
    # we can connect


def after_step(context, step):
    if BEHAVE_DEBUG_ON_ERROR and step.status == Status.failed:
        # Enter post-mortem debugging on test failure.
        import ipdb  # noqa: T100
        ipdb.post_mortem(step.exc_traceback)
