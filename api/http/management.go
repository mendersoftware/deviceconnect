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
	"strconv"
	"strings"
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
)

// HTTP errors
var (
	ErrMissingUserAuthentication = errors.New(
		"missing or non-user identity in the authorization headers",
	)
)

// ManagementController container for end-points
type ManagementController struct {
	app app.App
}

// NewManagementController returns a new ManagementController
func NewManagementController(app app.App) *ManagementController {
	return &ManagementController{app: app}
}

// GetDevice returns a device
func (h ManagementController) GetDevice(c *gin.Context) {
	ctx := c.Request.Context()

	idata := identity.FromContext(ctx)
	if idata == nil || !idata.IsUser {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": ErrMissingUserAuthentication.Error(),
		})
		return
	}
	tenantID := idata.Tenant
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
	}

	c.JSON(http.StatusOK, device)
}

// Connect starts a websocket connection with the device
func (h ManagementController) Connect(c *gin.Context) {
	ctx := c.Request.Context()
	l := log.FromContext(ctx)

	idata := identity.FromContext(ctx)
	if !idata.IsUser {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": ErrMissingUserAuthentication.Error(),
		})
		return
	}

	tenantID := idata.Tenant
	userID := idata.Subject
	deviceID := c.Param("deviceId")

	if len(c.Request.Header.Get(model.RBACHeaderRemoteTerminalGroups)) > 1 {
		groups := strings.Split(c.Request.Header.Get(model.RBACHeaderRemoteTerminalGroups), ",")

		allowed, err := h.app.RemoteTerminalAllowed(ctx, tenantID, deviceID, groups)
		if err != nil {
			l.Error(err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "internal error",
			})
			return
		} else if !allowed {
			c.JSON(http.StatusForbidden, gin.H{
				"error": "Access denied (RBAC).",
			})
			return
		}
	}

	// Prepare the user session
	session, err := h.app.PrepareUserSession(ctx, tenantID, userID, deviceID)
	if err == app.ErrDeviceNotFound || err == app.ErrDeviceNotConnected {
		c.JSON(http.StatusNotFound, gin.H{
			"error": err.Error(),
		})
		return
	} else if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	// upgrade get request to websocket protocol
	upgrader := websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		Subprotocols:    []string{"binary"},
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
		Error: func(
			w http.ResponseWriter, r *http.Request, s int, e error) {
			rest.RenderError(c, s, e)
		},
	}
	ws, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		err = errors.Wrap(err, "unable to upgrade the request to websocket protocol")
		l.Error(err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "internal error",
		})
		return
	}

	defer ws.Close()

	// handle the ping-pong connection health check
	err = ws.SetReadDeadline(time.Now().Add(pongWait * time.Second))
	if err != nil {
		l.Error(err)
		return
	}
	ws.SetPongHandler(func(string) error {
		return ws.SetReadDeadline(time.Now().Add(pongWait * time.Second))
	})

	// update the session status on websocket opening
	err = h.app.UpdateUserSessionStatus(
		ctx, tenantID,
		session.ID, model.SessiontatusConnected,
	)
	if err != nil {
		l.Error(err)
		return
	}
	defer func() {
		// update the device status on websocket closing
		err = h.app.UpdateUserSessionStatus(
			ctx, tenantID,
			session.ID, model.SessionStatusDisconnected,
		)
		if err != nil {
			l.Error(err)
		}
	}()

	// go-routine to read from the webservice
	done := make(chan struct{})
	go func() {
		var (
			err  error
			data []byte
		)
		for err == nil {
			_, data, err = ws.ReadMessage()
			if err != nil {
				break
			}
			m := &model.Message{}
			err = msgpack.Unmarshal(data, m)
			if err != nil {
				break
			}

			err = h.app.PublishMessageFromManagement(
				ctx, idata.Tenant, session.DeviceID, m,
			)
		}
		close(done)
	}()

	// subscribe to messages from the device
	messages, err := h.app.SubscribeMessagesFromDevice(ctx, idata.Tenant, session.DeviceID)
	if err != nil {
		return
	}

	websocketPing(ws)
	pingPeriod := (pongWait * time.Second * 9) / 10
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()

Loop:
	for {
		select {
		case message := <-messages:
			data, err := msgpack.Marshal(message)
			if err != nil {
				l.Error(err)
				break Loop
			}
			err = ws.WriteMessage(websocket.BinaryMessage, data)
			if err != nil {
				l.Error(err)
				break Loop
			}
		case <-done:
			break Loop
		case <-ticker.C:
			if !websocketPing(ws) {
				break Loop
			}
		}
	}
}

func websocketPing(conn *websocket.Conn) bool {
	pongWaitString := strconv.Itoa(pongWait)
	if err := conn.WriteControl(
		websocket.PingMessage,
		[]byte(pongWaitString),
		time.Now().Add(writeWait*time.Second),
	); err != nil {
		return false
	}
	return true
}
