import base64
import json
import os
import uuid
import pytest
import signal

import bson
import pymongo

import devices_api
import internal_api
import management_api


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
    devices_api.Configuration.set_default(
        devices_api.Configuration(
            host="http://" + host + "/api/devices/v1/deviceconnect"
        )
    )
    internal_api.Configuration.set_default(
        internal_api.Configuration(
            host="http://" + host + "/api/internal/v1/deviceconnect"
        )
    )
    management_api.Configuration.set_default(
        management_api.Configuration(
            host="http://" + host + "/api/management/v1/deviceconnect"
        )
    )


@pytest.fixture(scope="session")
def mongo():
    return pymongo.MongoClient("mongodb://mender-mongo")


def mongo_cleanup(client):
    dbs = client.list_database_names()
    for db in dbs:
        if db in ["local", "admin", "config"]:
            continue
        client.drop_database(db)


@pytest.fixture(scope="function")
def clean_mongo(mongo):
    mongo_cleanup(client=mongo)
    yield mongo
    mongo_cleanup(client=mongo)


@pytest.fixture(scope="function")
def tenant(tenant_id=None):
    """
    This fixture provisions a new tenant database.
    :param tenant_id: can be indirectly overridden with
                      @pytest.mark.fixture decorator.
    """
    if tenant_id is None:
        tenant_id = str(bson.objectid.ObjectId())
    client = internal_api.InternalAPIClient()
    client.provision_tenant(new_tenant=internal_api.NewTenant(tenant_id=tenant_id))
    yield tenant_id


def _test_timeout(signum, frame):
    raise TimeoutError("TestConnect did not finish in time")


@pytest.fixture(scope="function")
def timeout(request, timeout_sec=30):
    """"""
    alrm_handler = signal.getsignal(signal.SIGALRM)

    def timeout(signum, frame):
        raise TimeoutError("%s did not finish in time" % request.function.__name__)

    signal.signal(signal.SIGALRM, timeout)
    signal.alarm(timeout_sec)
    yield
    signal.signal(signal.SIGALRM, alrm_handler)
