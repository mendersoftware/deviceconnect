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
	"net/http"

	"github.com/gin-gonic/gin"
)

// DeviceConnectController container for end-points
type DeviceConnectController struct {
}

// NewDeviceConnectController returns a new StatusController
func NewDeviceConnectController() *DeviceConnectController {
	return &DeviceConnectController{}
}

// HealthCheck performs a health check and returns 204
func (h DeviceConnectController) HealthCheck(c *gin.Context) {
	c.Status(http.StatusNoContent)
}
