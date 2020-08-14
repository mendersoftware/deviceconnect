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
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mendersoftware/deviceconnect/app"
	"github.com/mendersoftware/deviceconnect/model"
	"github.com/pkg/errors"
)

// DeviceController container for end-points
type DeviceController struct {
	app app.App
}

// NewDeviceController returns a new DeviceController
func NewDeviceController(app app.App) *DeviceController {
	return &DeviceController{app: app}
}

// Provision responds to POST /tenants/:tenantId/devices
func (h DeviceController) Provision(c *gin.Context) {
	tenantID := c.Param("tenantId")

	rawData, err := c.GetRawData()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "bad request",
		})
		return
	}

	device := &model.Device{}
	if err = json.Unmarshal(rawData, device); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": errors.Wrap(err, "invalid payload").Error(),
		})
		return
	} else if device.DeviceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "device_id is empty",
		})
		return
	}

	ctx := c.Request.Context()
	if err = h.app.ProvisionDevice(ctx, tenantID, device); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": errors.Wrap(err, "error provisioning the device").Error(),
		})
		return
	}

	c.Writer.WriteHeader(http.StatusCreated)
}

// Delete responds to DELETE /tenants/:tenantId/devices/:deviceId
func (h DeviceController) Delete(c *gin.Context) {
	tenantID := c.Param("tenantId")
	deviceID := c.Param("deviceId")

	ctx := c.Request.Context()
	if err := h.app.DeleteDevice(ctx, tenantID, deviceID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": errors.Wrap(err, "error deleting the device").Error(),
		})
		return
	}

	c.Writer.WriteHeader(http.StatusAccepted)
}

// Connect starts a websocket
func (h DeviceController) Connect(c *gin.Context) {
	c.JSON(http.StatusOK, nil)
}
