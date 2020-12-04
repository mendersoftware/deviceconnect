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
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/pkg/errors"
	"github.com/vmihailenco/msgpack/v5"

	"github.com/mendersoftware/deviceconnect/app"
	"github.com/mendersoftware/deviceconnect/model"
	"github.com/mendersoftware/go-lib-micro/identity"
	"github.com/mendersoftware/go-lib-micro/log"
	"github.com/mendersoftware/go-lib-micro/rest.utils"
	"github.com/mendersoftware/go-lib-micro/ws"
)

const (
	// Time allowed to read the next pong message from the peer.
	pongWait = 60

	// Seconds allowed to write a message to the peer.
	writeWait = 10
)

// HTTP errors
var (
	ErrMissingAuthentication = errors.New(
		"missing or non-device identity in the authorization headers",
	)
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
	} else if device.ID == "" {
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

// Connect starts a websocket connection with the device
func (h DeviceController) Connect(c *gin.Context) {
	ctx := c.Request.Context()
	l := log.FromContext(ctx)

	idata := identity.FromContext(ctx)
	if !idata.IsDevice {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": ErrMissingAuthentication.Error(),
		})
		return
	}

	upgrader := websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		Subprotocols:    []string{"protomsg/msgpack"},
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
		Error: func(
			w http.ResponseWriter, r *http.Request, s int, e error) {
			rest.RenderError(c, s, e)
		},
	}

	// upgrade get request to websocket protocol
	webSock, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		err = errors.Wrap(err,
			"failed to upgrade the request to "+
				"websocket protocol",
		)
		l.Error(err)
		return
	}
	defer webSock.Close()

	// handle the ping-pong connection health check
	err = webSock.SetReadDeadline(time.Now().Add(pongWait * time.Second))
	if err != nil {
		l.Error(err)
		return
	}
	webSock.SetPongHandler(func(string) error {
		return webSock.SetReadDeadline(time.Now().Add(pongWait * time.Second))
	})

	// update the device status on websocket opening
	err = h.app.UpdateDeviceStatus(
		ctx, idata.Tenant,
		idata.Subject, model.DeviceStatusConnected,
	)
	if err != nil {
		l.Error(err)
		return
	}
	defer func() {
		// update the device status on websocket closing
		err = h.app.UpdateDeviceStatus(
			ctx, idata.Tenant,
			idata.Subject, model.DeviceStatusDisconnected,
		)
		if err != nil {
			l.Error(err)
		}
	}()

	// go-routine to read from the webservice
	done := make(chan struct{})
	go func() {
		var (
			data []byte
			err  error
		)
		for err == nil {
			_, data, err = webSock.ReadMessage()
			if err != nil {
				l.Error(err)
				break
			}
			m := &ws.ProtoMsg{}
			err = msgpack.Unmarshal(data, m)
			if err != nil {
				l.Error(err)
				break
			}
			err = h.app.PublishMessageFromDevice(ctx, idata.Tenant, idata.Subject, m)
		}
		close(done)
	}()

	// subscribe to messages from the device
	messages, err := h.app.SubscribeMessagesFromManagement(ctx, idata.Tenant, idata.Subject)
	if err != nil {
		return
	}

	// periodic ping
	websocketPing(webSock)
	pingPeriod := (pongWait * time.Second * 9) / 10
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()
	for {
		stop := false
		select {
		case message := <-messages:
			data, err := msgpack.Marshal(message)
			if err != nil {
				l.Fatal(err)
			}
			err = webSock.WriteMessage(websocket.BinaryMessage, data)
			if err != nil {
				l.Fatal(err)
			}
		case <-done:
			stop = true
			break
		case <-ticker.C:
			if !websocketPing(webSock) {
				stop = false
				break
			}
		}
		if stop {
			break
		}
	}
}
