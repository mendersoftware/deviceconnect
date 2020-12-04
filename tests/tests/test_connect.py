import json
import pytest
import uuid
from base64 import urlsafe_b64encode

import msgpack

import devices_api
import internal_api
import management_api

from common import Device, management_api_with_params, management_api_connect


class _TestConnect:
    def test_connect(self, clean_mongo, tenant_id=None):
        """
        Tests end-to-end connection between user and device and checks
        for basic expected responses.
        """

        dev = Device(tenant_id=tenant_id)
        api_mgmt = management_api_with_params(
            user_id=str(uuid.uuid4()), tenant_id=tenant_id
        )
        try:
            api_mgmt.connect(
                "00000000-0000-0000-0000-000000000000",
                connection="Upgrade",
                upgrade="websocket",
                sec_websocket_key="mumbojumbo==",
                sec_websocket_version=13,
            )
        except management_api.ApiException as e:
            assert e.status == 404
            assert "device not found" in str(e.body)
        else:
            raise Exception("Expected status code 404")

        try:
            api_mgmt.connect(
                dev.id,
                connection="Upgrade",
                upgrade="websocket",
                sec_websocket_key="mumbojumbo==",
                sec_websocket_version=13,
            )
        except management_api.ApiException as e:
            assert e.status == 404
            assert "device not connected" in str(e.body)
        else:
            raise Exception("Expected status code 404")

        with dev.connect() as dev_conn:
            try:
                api_mgmt.connect(dev.id)
            except management_api.ApiException as e:
                assert e.status == 400
                assert "the client is not using the websocket protocol" in e.body
            else:
                raise Exception("Expected status code 400")

            try:
                api_mgmt.connect(
                    dev.id,
                    connection="Upgrade",
                    upgrade="websocket",
                    sec_websocket_key="mumbojumbo==",
                    sec_websocket_version=13,
                )
            except management_api.ApiException as e:
                assert e.status == 101
            else:
                raise Exception("Expected status code 101")

            with management_api_connect(dev.id, tenant_id=tenant_id) as user_conn:
                user_conn.send(
                    msgpack.dumps(
                        {
                            "hdr": {
                                "proto": 1,
                                "typ": "shell",
                                "sid": "session-id",
                                "props": {"status": "ok"},
                            },
                            "body": None,
                        }
                    )
                )
                msg = dev_conn.recv()
                assert msgpack.loads(msg) == {
                    "hdr": {
                        "proto": 1,
                        "typ": "shell",
                        "sid": "session-id",
                        "props": {"status": "ok"},
                    },
                }
                dev_conn.send(
                    msgpack.dumps(
                        {
                            "hdr": {
                                "proto": 1,
                                "typ": "shell",
                                "sid": "session-id",
                                "props": {"status": "ok"},
                            },
                            "body": b"sh-5.0$ ",
                        }
                    )
                )
                msg = user_conn.recv()
                assert msgpack.loads(msg) == {
                    "hdr": {
                        "proto": 1,
                        "typ": "shell",
                        "sid": "session-id",
                        "props": {"status": "ok"},
                    },
                    "body": b"sh-5.0$ ",
                }

        try:
            api_mgmt.connect(
                dev.id,
                connection="Upgrade",
                upgrade="websocket",
                sec_websocket_key="mumbojumbo==",
                sec_websocket_version=13,
            )
        except management_api.ApiException as e:
            assert e.status == 404
        else:
            raise Exception("Expected status code 404")


class TestConnect(_TestConnect):
    def test_connect(self, clean_mongo):
        super().test_connect(clean_mongo)


class TestConnectEnterprise(_TestConnect):
    def test_connect(self, clean_mongo, tenant):
        super().test_connect(clean_mongo, tenant)
