// Copyright 2020 Northern.tech AS
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
	"context"

	"github.com/gin-gonic/gin"
	"github.com/mendersoftware/deviceconnect/app"
	"github.com/mendersoftware/go-lib-micro/log"
)

// API URL used by the HTTP router
const (
	APIURLDevices    = "/api/devices/v1/deviceconnect"
	APIURLInternal   = "/api/internal/v1/deviceconnect"
	APIURLManagement = "/api/management/v1/deviceconnect"

	APIURLDevicesConnect = APIURLDevices + "/connect"

	APIURLInternalAlive     = APIURLInternal + "/alive"
	APIURLInternalHealth    = APIURLInternal + "/health"
	APIURLInternalTenants   = APIURLInternal + "/tenants"
	APIURLInternalDevices   = APIURLInternal + "/tenants/:tenantId/devices"
	APIURLInternalDevicesID = APIURLInternal + "/tenants/:tenantId/devices/:deviceId"

	URLTerminal = "/"
)

// NewRouter returns the gin router
func NewRouter(deviceConnectApp app.App) (*gin.Engine, error) {
	gin.SetMode(gin.ReleaseMode)
	gin.DisableConsoleColor()

	ctx := context.Background()
	l := log.FromContext(ctx)

	router := gin.New()
	router.Use(IdentityMiddleware)
	router.Use(routerLogger(l))
	router.Use(gin.Recovery())

	status := NewStatusController(deviceConnectApp)
	router.GET(APIURLInternalAlive, status.Alive)
	router.GET(APIURLInternalHealth, status.Health)

	tenants := NewTenantsController(deviceConnectApp)
	router.POST(APIURLInternalTenants, tenants.Provision)

	device := NewDeviceController(deviceConnectApp)
	router.GET(APIURLDevicesConnect, device.Connect)
	router.POST(APIURLInternalDevices, device.Provision)
	router.DELETE(APIURLInternalDevicesID, device.Delete)

	terminal := NewTerminalController()
	router.GET(URLTerminal, terminal.Terminal)

	return router, nil
}
