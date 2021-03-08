// Copyright 2021 Northern.tech AS
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
	"github.com/mendersoftware/go-lib-micro/ws"
	"github.com/mendersoftware/go-lib-micro/ws/menderclient"
	"github.com/pkg/errors"
	"github.com/vmihailenco/msgpack/v5"

	"github.com/mendersoftware/deviceconnect/app"
	"github.com/mendersoftware/deviceconnect/client/nats"
	"github.com/mendersoftware/deviceconnect/model"
)

// InternalController contains status-related end-points
type InternalController struct {
	app  app.App
	nats nats.Client
}

// NewInternalController returns a new InternalController
func NewInternalController(app app.App, nc nats.Client) *InternalController {
	return &InternalController{app: app, nats: nc}
}

// Provision responds to POST /tenants
func (h InternalController) Provision(c *gin.Context) {
	rawData, err := c.GetRawData()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "bad request",
		})
		return
	}

	tenant := &model.Tenant{}
	if err = json.Unmarshal(rawData, tenant); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": errors.Wrap(err, "invalid payload").Error(),
		})
		return
	} else if tenant.TenantID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "tenant_id is empty",
		})
		return
	}

	ctx := c.Request.Context()
	if err = h.app.ProvisionTenant(ctx, tenant); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": errors.Wrap(err, "error provisioning the tenant").Error(),
		})
		return
	}

	c.Writer.WriteHeader(http.StatusCreated)
}

func (h InternalController) CheckUpdate(c *gin.Context) {
	h.sendMenderCommand(c, menderclient.MessageTypeMenderClientCheckUpdate)
}

func (h InternalController) SendInventory(c *gin.Context) {
	h.sendMenderCommand(c, menderclient.MessageTypeMenderClientSendInventory)
}

func (h InternalController) sendMenderCommand(c *gin.Context, msgType string) {
	ctx := c.Request.Context()

	tenantID := c.Param("tenantId")
	deviceID := c.Param("deviceId")

	device, err := h.app.GetDevice(ctx, tenantID, deviceID)
	if err == app.ErrDeviceNotFound {
		c.JSON(http.StatusNotFound, gin.H{
			"error": err.Error(),
		})
		return
	} else if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	} else if device.Status != model.DeviceStatusConnected {
		c.JSON(http.StatusConflict, gin.H{
			"error": app.ErrDeviceNotConnected,
		})
		return
	}

	msg := &ws.ProtoMsg{
		Header: ws.ProtoHdr{
			Proto:   ws.ProtoTypeMenderClient,
			MsgType: msgType,
		},
	}
	data, _ := msgpack.Marshal(msg)

	err = h.nats.Publish(model.GetDeviceSubject(tenantID, device.ID), data)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
	}

	c.JSON(http.StatusAccepted, nil)
}
