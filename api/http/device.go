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
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/pkg/errors"
	"github.com/vmihailenco/msgpack/v5"

	"github.com/mendersoftware/deviceconnect/app"
	"github.com/mendersoftware/deviceconnect/client/deviceauth"
	"github.com/mendersoftware/deviceconnect/model"
	"github.com/mendersoftware/go-lib-micro/identity"
	"github.com/mendersoftware/go-lib-micro/log"
)

const (
	// Time allowed to read the next pong message from the peer.
	pongWait = 60

	// Seconds allowed to write a message to the peer.
	writeWait = 10
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	Subprotocols:    []string{"binary"},
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// HTTP errors
var (
	ErrMissingAuthentication = errors.New("missing or non-device identity in the authorization headers")
)

// DeviceController container for end-points
type DeviceController struct {
	app        app.App
	deviceauth deviceauth.ClientInterface
}

// NewDeviceController returns a new DeviceController
func NewDeviceController(app app.App, deviceauth deviceauth.ClientInterface) *DeviceController {
	return &DeviceController{app: app, deviceauth: deviceauth}
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
	if idata == nil || !idata.IsDevice {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": ErrMissingAuthentication.Error(),
		})
		return
	}

	token := extractTokenFromRequest(c.Request)
	err := h.deviceauth.Verify(ctx, token, c.Request.Method, c.Request.RequestURI)
	if err != nil {
		code := deviceauth.GetHTTPStatusCodeFromError(err)
		c.JSON(code, gin.H{
			"error": err.Error(),
		})
		return
	}

	// upgrade get request to websocket protocol
	ws, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		err = errors.Wrap(err, "unable to upgrade the request to websocket protocol")
		l.Error(err)
		return
	}

	defer ws.Close()

	// handle the ping-pong connection health check
	ws.SetReadDeadline(time.Now().Add(pongWait * time.Second))
	ws.SetPongHandler(func(string) error {
		ws.SetReadDeadline(time.Now().Add(pongWait * time.Second))
		return nil
	})

	// update the device status on websocket opening
	h.app.UpdateDeviceStatus(ctx, idata.Tenant, idata.Subject, model.DeviceStatusConnected)

	// go-routine to read from the webservice
	done := make(chan struct{})
	go func() {
		for {
			_, data, err := ws.ReadMessage()
			if err != nil {
				close(done)
				return
			}
			m := &model.Message{}
			err = msgpack.Unmarshal(data, m)
			if err != nil {
				close(done)
				return
			}
			err = h.app.PublishMessageFromDevice(ctx, idata.Tenant, idata.Subject, m)
			if err != nil {
				close(done)
				return
			}
		}
	}()

	// subscribe to messages from the device
	messages, err := h.app.SubscribeMessagesFromManagement(ctx, idata.Tenant, idata.Subject)
	if err != nil {
		return
	}

	// periodic ping
	sendPing := func() bool {
		pongWaitString := strconv.Itoa(pongWait)
		if err := ws.WriteControl(websocket.PingMessage, []byte(pongWaitString), time.Now().Add(writeWait*time.Second)); err != nil {
			return false
		}
		return true
	}
	sendPing()

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
			ws.WriteMessage(websocket.BinaryMessage, data)
		case <-done:
			stop = true
			break
		case <-ticker.C:
			if !sendPing() {
				stop = false
				break
			}
		}
		if stop {
			break
		}
	}

	// update the device status on websocket closing
	h.app.UpdateDeviceStatus(ctx, idata.Tenant, idata.Subject, model.DeviceStatusDisconnected)
}
