// Copyright 2022 Northern.tech AS
//
//    Licensed under the Apache License, Version 2.0 (the "License");
//    you may not use this file except in compliance with the License.
//    You may obtain a copy of the License at
//
//        http://www.apache.org/licenses/LICENSE-2.0
//
//    Unless required by applicable law or agreed to in writing, software
//    distributed under the License is distributed on an "AS IS" BASIS,
//    WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//    See the License for the specific language governing permissions and
//    limitations under the License.

package http

import (
	"github.com/gin-gonic/gin"

	"github.com/mendersoftware/go-lib-micro/accesslog"
	"github.com/mendersoftware/go-lib-micro/identity"
	"github.com/mendersoftware/go-lib-micro/requestid"

	"github.com/mendersoftware/deviceconnect/app"
	"github.com/mendersoftware/deviceconnect/client/nats"
)

// API URL used by the HTTP router
const (
	APIURLDevices    = "/api/devices/v1/deviceconnect"
	APIURLInternal   = "/api/internal/v1/deviceconnect"
	APIURLManagement = "/api/management/v1/deviceconnect"

	APIURLDevicesConnect = APIURLDevices + "/connect"

	APIURLInternalAlive     = APIURLInternal + "/alive"
	APIURLInternalHealth    = APIURLInternal + "/health"
	APIURLInternalDevices   = APIURLInternal + "/tenants/:tenantId/devices"
	APIURLInternalDevicesID = APIURLInternal +
		"/tenants/:tenantId/devices/:deviceId"
	APIURLInternalDevicesIDCheckUpdate = APIURLInternal +
		"/tenants/:tenantId/devices/:deviceId/check-update"
	APIURLInternalDevicesIDSendInventory = APIURLInternal +
		"/tenants/:tenantId/devices/:deviceId/send-inventory"

	APIURLManagementDevice              = APIURLManagement + "/devices/:deviceId"
	APIURLManagementDeviceConnect       = APIURLManagement + "/devices/:deviceId/connect"
	APIURLManagementDeviceDownload      = APIURLManagement + "/devices/:deviceId/download"
	APIURLManagementDeviceCheckUpdate   = APIURLManagement + "/devices/:deviceId/check-update"
	APIURLManagementDeviceSendInventory = APIURLManagement + "/devices/:deviceId/send-inventory"
	APIURLManagementDeviceUpload        = APIURLManagement + "/devices/:deviceId/upload"
	APIURLManagementPlayback            = APIURLManagement + "/sessions/:sessionId/playback"
)

// NewRouter returns the gin router
func NewRouter(
	app app.App,
	natsClient nats.Client,
) (*gin.Engine, error) {
	gin.SetMode(gin.ReleaseMode)
	gin.DisableConsoleColor()

	router := gin.New()
	router.Use(accesslog.Middleware())
	router.Use(gin.Recovery())
	router.Use(identity.Middleware(
		identity.NewMiddlewareOptions().
			SetPathRegex(`^/api/(devices|management)/v[0-9]/`),
	))
	router.Use(requestid.Middleware())

	status := NewStatusController(app)
	router.GET(APIURLInternalAlive, status.Alive)
	router.GET(APIURLInternalHealth, status.Health)

	internal := NewInternalController(app, natsClient)
	router.POST(APIURLInternalDevicesIDCheckUpdate, internal.CheckUpdate)
	router.POST(APIURLInternalDevicesIDSendInventory, internal.SendInventory)

	device := NewDeviceController(app, natsClient)
	router.GET(APIURLDevicesConnect, device.Connect)
	router.POST(APIURLInternalDevices, device.Provision)
	router.DELETE(APIURLInternalDevicesID, device.Delete)

	management := NewManagementController(app, natsClient)
	router.GET(APIURLManagementDevice, management.GetDevice)
	router.GET(APIURLManagementDeviceConnect, management.Connect)
	router.GET(APIURLManagementDeviceDownload, management.DownloadFile)
	router.POST(APIURLManagementDeviceCheckUpdate, management.CheckUpdate)
	router.POST(APIURLManagementDeviceSendInventory, management.SendInventory)
	router.PUT(APIURLManagementDeviceUpload, management.UploadFile)
	router.GET(APIURLManagementPlayback, management.Playback)

	return router, nil
}
