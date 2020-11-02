import os

from devices_api import configuration as dconf
from internal_api import configuration as iconf
from management_api import configuration as mconf


def pytest_addoption(parser):
    parser.addoption(
        "--host",
        action="store",
        default=os.environ["TESTING_HOST"]
        if "TESTING_HOST" in os.environ
        else "localhost",
        help="Address for host hosting deviceconnect API (env: TEST_HOST)",
    )


def pytest_configure(config):
    host = config.getoption("host")
    dconf.Configuration.set_default(
        dconf.Configuration(host="http://" + host + "/api/devices/v1/deviceconnect")
    )
    iconf.Configuration.set_default(
        iconf.Configuration(host="http://" + host + "/api/internal/v1/deviceconnect")
    )
    mconf.Configuration.set_default(
        mconf.Configuration(host="http://" + host + "/api/management/v1/deviceconnect")
    )
