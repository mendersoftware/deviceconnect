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
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/pkg/errors"
	"github.com/vmihailenco/msgpack/v5"

	"github.com/mendersoftware/deviceconnect/app"
	"github.com/mendersoftware/deviceconnect/model"
	"github.com/mendersoftware/go-lib-micro/identity"
	"github.com/mendersoftware/go-lib-micro/log"
)

// HTTP errors
var (
	ErrMissingUserAuthentication = errors.New("missing or non-useer identity in the authorization headers")
)

// ManagementController container for end-points
type ManagementController struct {
	app app.App
}

// NewManagementController returns a new ManagementController
func NewManagementController(app app.App) *ManagementController {
	return &ManagementController{app: app}
}

// Connect starts a websocket connection with the device
func (h ManagementController) Connect(c *gin.Context) {
	ctx := c.Request.Context()
	l := log.FromContext(ctx)

	idata := identity.FromContext(ctx)
	if idata == nil || !idata.IsUser {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": ErrMissingUserAuthentication.Error(),
		})
		return
	}
	tenantID := idata.Tenant
	userID := idata.Subject
	deviceID := c.Param("deviceId")

	// Prepare the user session
	session, err := h.app.PrepareUserSession(ctx, tenantID, userID, deviceID)
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

	// update the session status on websocket opening
	h.app.UpdateUserSessionStatus(ctx, tenantID, session.ID, model.SessiontatusConnected)

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
			l.Printf("received: %s / %s", m.Cmd, m.Data)
		}
	}()

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
	h.app.UpdateUserSessionStatus(ctx, tenantID, session.ID, model.SessionStatusDisconnected)
}
