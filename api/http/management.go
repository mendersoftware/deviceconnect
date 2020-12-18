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
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	validation "github.com/go-ozzo/ozzo-validation/v4"
	"github.com/gorilla/websocket"
	"github.com/nats-io/nats.go"
	"github.com/pkg/errors"
	"github.com/vmihailenco/msgpack/v5"

	"github.com/mendersoftware/deviceconnect/app"
	"github.com/mendersoftware/deviceconnect/model"
	"github.com/mendersoftware/go-lib-micro/identity"
	"github.com/mendersoftware/go-lib-micro/log"
	"github.com/mendersoftware/go-lib-micro/rest.utils"
	"github.com/mendersoftware/go-lib-micro/ws"
	"github.com/mendersoftware/go-lib-micro/ws/shell"
)

// HTTP errors
var (
	ErrMissingUserAuthentication = errors.New(
		"missing or non-user identity in the authorization headers",
	)
)

const channelSize = 25 // TODO make configurable

const (
	PropertyUserID = "user_id"
)

// ManagementController container for end-points
type ManagementController struct {
	app  app.App
	nats *nats.Conn
}

// NewManagementController returns a new ManagementController
func NewManagementController(
	app app.App,
	nc *nats.Conn,
) *ManagementController {
	return &ManagementController{
		app:  app,
		nats: nc,
	}
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

// Connect extracts identity from request, checks user permissions
// and calls ConnectDevice
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
		groups := strings.Split(
			c.Request.Header.Get(model.RBACHeaderRemoteTerminalGroups), ",")

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

	session := &model.Session{
		TenantID: tenantID,
		UserID:   userID,
		DeviceID: deviceID,
		StartTS:  time.Now(),
	}

	// Prepare the user session
	err := h.app.PrepareUserSession(ctx, session)
	if err == app.ErrDeviceNotFound || err == app.ErrDeviceNotConnected {
		c.JSON(http.StatusNotFound, gin.H{
			"error": err.Error(),
		})
		return
	} else if _, ok := errors.Cause(err).(validation.Errors); ok {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	} else if err != nil {
		l.Error(err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}
	defer func() {
		err := h.app.FreeUserSession(ctx, session.ID)
		if err != nil {
			l.Warnf("failed to free session: %s", err.Error())
		}
	}()

	deviceChan := make(chan *nats.Msg, channelSize)
	sub, err := h.nats.ChanSubscribe(session.Subject(tenantID), deviceChan)
	if err != nil {
		l.Error(err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to establish internal device session",
		})
		return
	}
	//nolint:errcheck
	defer sub.Unsubscribe()

	// upgrade get request to websocket protocol
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

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		err = errors.Wrap(err, "unable to upgrade the request to websocket protocol")
		l.Error(err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "internal error",
		})
		return
	}

	//nolint:errcheck
	h.ConnectServeWS(ctx, conn, session, deviceChan)
}

func websocketPing(conn *websocket.Conn) bool {
	pongWaitString := strconv.Itoa(int(pongWait.Seconds()))
	if err := conn.WriteControl(
		websocket.PingMessage,
		[]byte(pongWaitString),
		time.Now().Add(writeWait),
	); err != nil {
		return false
	}
	return true
}

// websocketWriter is the go-routine responsible for the writing end of the
// websocket. The routine forwards messages posted on the NATS session subject
// and periodically pings the connection. If the connection times out or a
// protocol violation occurs, the routine closes the connection.
func (h ManagementController) websocketWriter(
	ctx context.Context,
	conn *websocket.Conn,
	session *model.Session,
	deviceChan <-chan *nats.Msg,
	errChan <-chan error,
) (err error) {
	l := log.FromContext(ctx)
	defer func() {
		// TODO Check errChan and send close control packet with error
		// if not initiated by client.
		conn.Close()
	}()

	// handle the ping-pong connection health check
	err = conn.SetReadDeadline(time.Now().Add(pongWait))
	if err != nil {
		l.Error(err)
		return err
	}

	pingPeriod := (pongWait * 9) / 10
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()
	conn.SetPongHandler(func(string) error {
		ticker.Reset(pingPeriod)
		return conn.SetReadDeadline(time.Now().Add(pongWait))
	})
	conn.SetPingHandler(func(msg string) error {
		ticker.Reset(pingPeriod)
		err := conn.SetReadDeadline(time.Now().Add(pongWait))
		if err != nil {
			return err
		}
		return conn.WriteControl(
			websocket.PongMessage,
			[]byte(msg),
			time.Now().Add(writeWait),
		)
	})
Loop:
	for {
		select {
		case msg := <-deviceChan:
			err = conn.WriteMessage(websocket.BinaryMessage, msg.Data)
			if err != nil {
				l.Error(err)
				break Loop
			}
		case <-ctx.Done():
			break Loop
		case <-ticker.C:
			if !websocketPing(conn) {
				break Loop
			}
		case err := <-errChan:
			return err
		}
		ticker.Reset(pingPeriod)
	}
	return err
}

// ConnectServeWS starts a websocket connection with the device
// Currently this handler only properly handles a single terminal session.
func (h ManagementController) ConnectServeWS(
	ctx context.Context,
	conn *websocket.Conn,
	sess *model.Session,
	deviceChan chan *nats.Msg,
) (err error) {
	var sessionClosed bool
	l := log.FromContext(ctx)
	id := identity.FromContext(ctx)
	errChan := make(chan error, 1)
	defer func() {
		if err != nil {
			select {
			case errChan <- err:

			case <-time.After(time.Second):
				l.Warn("Failed to propagate error to client")
			}
		}
		if !sessionClosed {
			msg := ws.ProtoMsg{
				Header: ws.ProtoHdr{
					Proto:     ws.ProtoTypeShell,
					MsgType:   shell.MessageTypeStopShell,
					SessionID: sess.ID,
					Properties: map[string]interface{}{
						"status":       shell.ErrorMessage,
						PropertyUserID: sess.UserID,
					},
				},
				Body: []byte("user disconnected"),
			}
			data, _ := msgpack.Marshal(msg)
			errPublish := h.nats.Publish(model.GetDeviceSubject(
				id.Tenant, sess.DeviceID),
				data,
			)
			if errPublish != nil {
				l.Warnf(
					"failed to propagate stop session "+
						"message to device: %s",
					errPublish.Error(),
				)
			}
		}
		close(errChan)
	}()
	// websocketWriter is responsible for closing the websocket
	//nolint:errcheck
	go h.websocketWriter(ctx, conn, sess, deviceChan, errChan)

	var data []byte
	for {
		_, data, err = conn.ReadMessage()
		if err != nil {
			if _, ok := err.(*websocket.CloseError); ok {
				return nil
			}
			return err
		}
		m := &ws.ProtoMsg{}
		err = msgpack.Unmarshal(data, m)
		if err != nil {
			return err
		}

		switch m.Header.Proto {
		case ws.ProtoTypeShell:
			m.Header.SessionID = sess.ID
			if m.Header.Properties == nil {
				m.Header.Properties = make(map[string]interface{})
			}
			m.Header.Properties[PropertyUserID] = sess.UserID
			data, _ = msgpack.Marshal(m)

			if m.Header.MsgType == shell.MessageTypeStopShell {
				sessionClosed = true
			}
		default:
			// TODO: Handle protocol violation
		}

		err = h.nats.Publish(model.GetDeviceSubject(id.Tenant, sess.DeviceID), data)
		if err != nil {
			return err
		}
	}
}
