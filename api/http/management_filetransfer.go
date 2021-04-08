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
	"fmt"
	"io"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	natsio "github.com/nats-io/nats.go"
	"github.com/pkg/errors"
	"github.com/vmihailenco/msgpack/v5"

	"github.com/mendersoftware/go-lib-micro/identity"
	"github.com/mendersoftware/go-lib-micro/log"
	"github.com/mendersoftware/go-lib-micro/ws"
	wsft "github.com/mendersoftware/go-lib-micro/ws/filetransfer"

	"github.com/mendersoftware/deviceconnect/app"
	"github.com/mendersoftware/deviceconnect/model"
)

type fileTransferParams struct {
	TenantID  string
	UserID    string
	SessionID string
	Device    *model.Device
}

const (
	hdrContentType            = "Content-Type"
	hdrContentDisposition     = "Content-Disposition"
	hdrMenderFileTransferPath = "X-MEN-File-Path"
	hdrMenderFileTransferUID  = "X-MEN-File-UID"
	hdrMenderFileTransferGID  = "X-MEN-File-GID"
	hdrMenderFileTransferMode = "X-MEN-File-Mode"
	hdrMenderFileTransferSize = "X-MEN-File-Size"
)

const (
	fieldUploadPath = "path"
	fieldUploadUID  = "uid"
	fieldUploadGID  = "gid"
	fieldUploadMode = "mode"
	fieldUploadFile = "file"

	PropertyOffset = "offset"

	paramDownloadPath = "path"
)

var fileTransferPingInterval = 30 * time.Second
var fileTransferTimeout = 60 * time.Second
var fileTransferBufferSize = 4096
var ackSlidingWindowSend = 10
var ackSlidingWindowRecv = 20

var (
	errFileTransferMarshalling    = errors.New("failed to marshal the request")
	errFileTransferUnmarshalling  = errors.New("failed to unmarshal the request")
	errFileTransferPublishing     = errors.New("failed to publish the message")
	errFileTransferSubscribing    = errors.New("failed to subscribe to the mesages")
	errFileTransferTimeout        = errors.New("file transfer timed out")
	errFileTransferFailed         = errors.New("file transfer failed")
	errFileTransferNotImplemented = errors.New("file transfer not implemented on device")
	errFileTransferDisabled       = errors.New("file transfer disabled on device")
)

var newFileTransferSessionID = func() (uuid.UUID, error) {
	return uuid.NewRandom()
}

func (h ManagementController) getFileTransferParams(c *gin.Context) (*fileTransferParams, int,
	error) {
	ctx := c.Request.Context()

	idata := identity.FromContext(ctx)
	if idata == nil || !idata.IsUser {
		return nil, http.StatusUnauthorized, ErrMissingUserAuthentication
	}
	tenantID := idata.Tenant
	deviceID := c.Param("deviceId")

	device, err := h.app.GetDevice(ctx, tenantID, deviceID)
	if err == app.ErrDeviceNotFound {
		return nil, http.StatusNotFound, err
	} else if err != nil {
		return nil, http.StatusBadRequest, err
	} else if device.Status != model.DeviceStatusConnected {
		return nil, http.StatusConflict, app.ErrDeviceNotConnected
	}

	if c.Request.Method != http.MethodGet && c.Request.Body == nil {
		return nil, http.StatusBadRequest, errors.New("missing request body")
	}

	sessionID, err := newFileTransferSessionID()
	if err != nil {
		return nil, http.StatusInternalServerError,
			errors.New("failed to generate session ID")
	}

	return &fileTransferParams{
		TenantID:  idata.Tenant,
		UserID:    idata.Subject,
		SessionID: sessionID.String(),
		Device:    device,
	}, 0, nil
}

func (h ManagementController) publishFileTransferProtoMessage(sessionID, userID, deviceTopic,
	msgType string, body interface{}, offset int64) error {
	var msgBody []byte
	if msgType == wsft.MessageTypeChunk && body != nil {
		msgBody = body.([]byte)
	} else if msgType == wsft.MessageTypeACK {
		msgBody = nil
	} else if body != nil {
		var err error
		msgBody, err = msgpack.Marshal(body)
		if err != nil {
			return errors.Wrap(err, errFileTransferMarshalling.Error())
		}
	}
	proto := ws.ProtoTypeFileTransfer
	if msgType == ws.MessageTypePing || msgType == ws.MessageTypePong {
		proto = ws.ProtoTypeControl
	}
	msg := &ws.ProtoMsg{
		Header: ws.ProtoHdr{
			Proto:     proto,
			MsgType:   msgType,
			SessionID: sessionID,
			Properties: map[string]interface{}{
				PropertyUserID: userID,
			},
		},
		Body: msgBody,
	}
	if msgType == wsft.MessageTypeChunk || msgType == wsft.MessageTypeACK {
		msg.Header.Properties[PropertyOffset] = offset
	}
	data, err := msgpack.Marshal(msg)
	if err != nil {
		return errors.Wrap(err, errFileTransferMarshalling.Error())
	}

	err = h.nats.Publish(deviceTopic, data)
	if err != nil {
		return errors.Wrap(err, errFileTransferPublishing.Error())
	}
	return nil
}

func (h ManagementController) publishControlMessage(
	sessionID, deviceTopic, messageType string, body interface{},
) error {
	msg := &ws.ProtoMsg{
		Header: ws.ProtoHdr{
			Proto:     ws.ProtoTypeControl,
			MsgType:   messageType,
			SessionID: sessionID,
		},
	}

	if body != nil {
		if b, ok := body.([]byte); ok {
			msg.Body = b
		} else {
			b, err := msgpack.Marshal(body)
			if err != nil {
				return errors.Wrap(errFileTransferMarshalling, err.Error())
			}
			msg.Body = b
		}
	}

	data, err := msgpack.Marshal(msg)
	if err != nil {
		return errors.Wrap(errFileTransferMarshalling, err.Error())
	}
	err = h.nats.Publish(deviceTopic, data)
	if err != nil {
		return errors.Wrap(errFileTransferPublishing, err.Error())
	}
	return err
}

func (h ManagementController) decodeFileTransferProtoMessage(data []byte) (*ws.ProtoMsg,
	interface{}, error) {
	msg := &ws.ProtoMsg{}
	err := msgpack.Unmarshal(data, msg)
	if err != nil {
		return nil, nil, errors.Wrap(err, errFileTransferUnmarshalling.Error())
	}

	switch msg.Header.MsgType {
	case wsft.MessageTypeError:
		msgBody := &wsft.Error{}
		err := msgpack.Unmarshal(msg.Body, msgBody)
		if err != nil {
			return nil, nil, errors.Wrap(err, errFileTransferUnmarshalling.Error())
		}
		return msg, msgBody, nil
	case wsft.MessageTypeFileInfo:
		msgBody := &wsft.FileInfo{}
		err := msgpack.Unmarshal(msg.Body, msgBody)
		if err != nil {
			return nil, nil, errors.Wrap(err, errFileTransferUnmarshalling.Error())
		}
		return msg, msgBody, nil
	case wsft.MessageTypeACK, wsft.MessageTypeChunk, ws.MessageTypePing, ws.MessageTypePong:
		return msg, nil, nil
	}

	return nil, nil, errors.Errorf("unexpected message type '%s'", msg.Header.MsgType)
}

func writeHeaders(c *gin.Context, fileInfo *wsft.FileInfo) {
	c.Writer.WriteHeader(http.StatusOK)
	c.Writer.Header().Add(hdrContentType, "application/octet-stream")
	if fileInfo.Path != nil {
		filename := path.Base(*fileInfo.Path)
		c.Writer.Header().Add(hdrContentDisposition,
			"attachment; filename=\""+filename+"\"")
		c.Writer.Header().Add(hdrMenderFileTransferPath, *fileInfo.Path)
	}
	if fileInfo.UID != nil {
		c.Writer.Header().Add(hdrMenderFileTransferUID, fmt.Sprintf("%d", *fileInfo.UID))
	}
	if fileInfo.GID != nil {
		c.Writer.Header().Add(hdrMenderFileTransferGID, fmt.Sprintf("%d", *fileInfo.GID))
	}
	if fileInfo.Mode != nil {
		c.Writer.Header().Add(hdrMenderFileTransferMode, fmt.Sprintf("%o", *fileInfo.Mode))
	}
	if fileInfo.Size != nil {
		c.Writer.Header().Add(hdrMenderFileTransferSize, fmt.Sprintf("%d", *fileInfo.Size))
	}
}

func (h ManagementController) downloadFileResponseError(c *gin.Context,
	responseHeaderSent *bool, responseError *error) {
	l := log.FromContext(c.Request.Context())
	if !*responseHeaderSent && *responseError != nil {
		l.Error((*responseError).Error())
		status := http.StatusInternalServerError
		// errFileTranserFailed is a special case, we return 400 instead of 500
		if strings.Contains((*responseError).Error(), errFileTransferFailed.Error()) {
			status = http.StatusBadRequest
		} else if *responseError == errFileTransferTimeout {
			status = http.StatusRequestTimeout
		} else if *responseError == errFileTransferNotImplemented ||
			*responseError == errFileTransferDisabled {
			status = http.StatusBadGateway
		}
		c.JSON(status, gin.H{
			"error": (*responseError).Error(),
		})
		return
	}
}

func (h ManagementController) downloadFileResponse(c *gin.Context, params *fileTransferParams,
	request *model.DownloadFileRequest) {
	// send a JSON-encoded error message in case of failure
	var responseError error
	var responseHeaderSent bool
	defer h.downloadFileResponseError(c, &responseHeaderSent, &responseError)

	// subscribe to messages from the device
	deviceTopic := model.GetDeviceSubject(params.TenantID, params.Device.ID)
	sessionTopic := model.GetSessionSubject(params.TenantID, params.SessionID)
	msgChan := make(chan *natsio.Msg, channelSize)
	sub, err := h.nats.ChanSubscribe(sessionTopic, msgChan)
	if err != nil {
		responseError = errors.Wrap(err, errFileTransferSubscribing.Error())
		return
	}

	if err = h.filetransferHandshake(msgChan, params.SessionID, deviceTopic); err != nil {
		responseError = err
		return
	}

	//nolint:errcheck
	defer sub.Unsubscribe()

	// stat the remote file
	req := wsft.StatFile{
		Path: request.Path,
	}
	if err := h.publishFileTransferProtoMessage(params.SessionID,
		params.UserID, deviceTopic, wsft.MessageTypeStat, req, 0); err != nil {
		responseError = err
		return
	}

	// Inform the device that we're closing the session
	//nolint:errcheck
	defer h.publishControlMessage(params.SessionID, deviceTopic, ws.MessageTypeClose, nil)

	ticker := time.NewTicker(fileTransferPingInterval)
	defer ticker.Stop()

	// handle messages from the device
	timeout := time.NewTimer(fileTransferTimeout)
	latestOffset := int64(0)
	numberOfChunks := 0
	for {
		select {
		case wsMessage := <-msgChan:
			// reset the timeout ticket
			timeout.Reset(fileTransferTimeout)
			// process the message
			err := h.downloadFileResponseProcessMessage(c, params, request,
				wsMessage, deviceTopic, &latestOffset, &numberOfChunks,
				&responseHeaderSent, ticker)
			if err == io.EOF {
				return
			} else if err != nil {
				responseError = err
				return
			}
		// send a Ping message to keep the session alive
		case <-ticker.C:
			responseError = h.publishControlMessage(
				params.SessionID, deviceTopic, ws.MessageTypePing, nil,
			)
			if responseError != nil {
				return
			}

		// no message after timeout expired, stop here
		case <-timeout.C:
			responseError = errFileTransferTimeout
			return
		}
	}
}

func (h ManagementController) downloadFileResponseProcessMessage(c *gin.Context,
	params *fileTransferParams, request *model.DownloadFileRequest, wsMessage *natsio.Msg,
	deviceTopic string, latestOffset *int64, numberOfChunks *int, responseHeaderSent *bool,
	ticker *time.Ticker) error {
	msg, msgBody, err := h.decodeFileTransferProtoMessage(wsMessage.Data)
	if err != nil {
		return err
	}

	// process incoming messages from the device by type
	switch msg.Header.MsgType {

	// error message, stop here
	case wsft.MessageTypeError:
		errorMsg := msgBody.(*wsft.Error)
		if *errorMsg.MessageType == wsft.MessageTypeStat {
			return errors.Wrap(errors.New(*errorMsg.Error),
				errFileTransferFailed.Error())
		} else {
			return errors.New(*errorMsg.Error)
		}

	// file stat response, if okay, let's get the file
	case wsft.MessageTypeFileInfo:
		req := wsft.GetFile{
			Path: request.Path,
		}
		if err := h.publishFileTransferProtoMessage(params.SessionID,
			params.UserID, deviceTopic, wsft.MessageTypeGet,
			req, 0); err != nil {
			return err
		}

		fileInfo := msgBody.(*wsft.FileInfo)
		writeHeaders(c, fileInfo)
		*responseHeaderSent = true

	// file data chunk
	case wsft.MessageTypeChunk:
		if msg.Body == nil {
			if err := h.publishFileTransferProtoMessage(
				params.SessionID, params.UserID, deviceTopic,
				wsft.MessageTypeACK, nil,
				*latestOffset); err != nil {
				return err
			}
			return io.EOF
		}

		// verify the offset property
		propOffset, _ := msg.Header.Properties[PropertyOffset].(int64)
		if propOffset != *latestOffset {
			return errors.Wrap(errFileTransferFailed,
				"wrong offset received")
		}
		*latestOffset += int64(len(msg.Body))

		_, err := c.Writer.Write(msg.Body)
		if err != nil {
			return err
		}

		(*numberOfChunks)++
		if *numberOfChunks >= ackSlidingWindowSend {
			if err := h.publishFileTransferProtoMessage(
				params.SessionID, params.UserID, deviceTopic,
				wsft.MessageTypeACK, nil,
				*latestOffset); err != nil {
				return err
			}
			*numberOfChunks = 0
		}

	case ws.MessageTypePing:
		if err := h.publishFileTransferProtoMessage(
			params.SessionID, params.UserID, deviceTopic,
			ws.MessageTypePong, nil,
			-1); err != nil {
			return err
		}
		fallthrough

	case ws.MessageTypePong:
		ticker.Reset(fileTransferPingInterval)
	}

	return nil
}

func (h ManagementController) DownloadFile(c *gin.Context) {
	l := log.FromContext(c.Request.Context())

	params, statusCode, err := h.getFileTransferParams(c)
	if err != nil {
		l.Error(err)
		c.JSON(statusCode, gin.H{"error": err.Error()})
		return
	}

	path := c.Request.URL.Query().Get(paramDownloadPath)
	request := &model.DownloadFileRequest{
		Path: &path,
	}

	if err := request.Validate(); err != nil {
		l.Error(err)
		c.JSON(http.StatusBadRequest, gin.H{
			"error": errors.Wrap(err, "bad request").Error(),
		})
		return
	}

	allowed, err := h.fileTransferAllowed(c, params.TenantID, params.Device.ID)
	if err != nil {
		l.Error(err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": errors.Wrap(err, "failed to check RBAC"),
		})
		return
	} else if !allowed {
		msg := "Access denied (RBAC)."
		l.Warn(msg)
		c.JSON(http.StatusForbidden, gin.H{
			"error": msg,
		})
		return
	}

	ctx := c.Request.Context()
	if err := h.app.DownloadFile(ctx, params.UserID, params.Device.ID,
		*request.Path); err != nil {
		l.Error(err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": errors.Wrap(err, "bad request").Error(),
		})
		return
	}

	h.downloadFileResponse(c, params, request)
}

func (h ManagementController) uploadFileResponseHandleInboundMessages(
	c *gin.Context, params *fileTransferParams,
	msgChan chan *natsio.Msg, errorChan chan error,
	latestAckOffsets chan int64,
) {
	var latestAckOffset int64
	deviceTopic := model.GetDeviceSubject(params.TenantID, params.Device.ID)
	for {
		select {
		case wsMessage := <-msgChan:
			msg, msgBody, err := h.decodeFileTransferProtoMessage(
				wsMessage.Data)
			if err != nil {
				errorChan <- err
				return
			}

			// process incoming messages from the device by type
			switch msg.Header.MsgType {

			// error message, stop here
			case wsft.MessageTypeError:
				errorMsg := msgBody.(*wsft.Error)
				errorChan <- errors.New(*errorMsg.Error)
				return

			// you can continue the upload
			case wsft.MessageTypeACK:
				propValue := msg.Header.Properties[PropertyOffset]
				propOffset, _ := propValue.(int64)
				if propOffset > latestAckOffset {
					latestAckOffset = propOffset
					select {
					case latestAckOffsets <- latestAckOffset:
					case <-latestAckOffsets:
						// Replace ack offset with the latest one
						latestAckOffsets <- latestAckOffset
					}
				}

			// handle ping messages
			case ws.MessageTypePing:
				if err := h.publishFileTransferProtoMessage(
					params.SessionID, params.UserID, deviceTopic,
					ws.MessageTypePong, nil,
					-1); err != nil {
					errorChan <- err
				}
			}
		case <-c.Done():
			return
		}
	}
}

// filetransferHandshake initiates a handshake and checks that the device
// is willing to accept file transfer requests.
func (h ManagementController) filetransferHandshake(
	sessChan <-chan *natsio.Msg, sessionID, deviceTopic string,
) error {
	if err := h.publishControlMessage(
		sessionID, deviceTopic,
		ws.MessageTypeOpen, ws.Open{
			Versions: []int{ws.ProtocolVersion},
		}); err != nil {
		return errFileTransferPublishing
	}
	select {
	case natsMsg := <-sessChan:
		var msg ws.ProtoMsg
		err := msgpack.Unmarshal(natsMsg.Data, &msg)
		if err != nil {
			return errFileTransferUnmarshalling
		}

		if msg.Header.MsgType == ws.MessageTypeError {
			erro := new(ws.Error)
			//nolint:errcheck
			msgpack.Unmarshal(natsMsg.Data, erro)
			return errors.Errorf("handshake error from client: %s", erro.Error)
		} else if msg.Header.MsgType != ws.MessageTypeAccept {
			return errFileTransferNotImplemented
		}
		accept := new(ws.Accept)
		err = msgpack.Unmarshal(msg.Body, accept)
		if err != nil {
			return errFileTransferUnmarshalling
		}

		for _, proto := range accept.Protocols {
			if proto == ws.ProtoTypeFileTransfer {
				return nil
			}
		}
		// Let's try to be polite and close the session before returning
		//nolint:errcheck
		h.publishControlMessage(sessionID, deviceTopic, ws.MessageTypeClose, nil)
		return errFileTransferDisabled

	case <-time.After(fileTransferTimeout):
		return errFileTransferTimeout
	}
}

func (h ManagementController) uploadFileResponse(c *gin.Context, params *fileTransferParams,
	request *model.UploadFileRequest) {
	l := log.FromContext(c.Request.Context())

	// send a JSON-encoded error message in case of failure
	var responseError error
	errorStatusCode := http.StatusInternalServerError
	defer func() {
		if responseError != nil {
			l.Error(responseError.Error())
			c.JSON(errorStatusCode, gin.H{
				"error": responseError.Error(),
			})
			return
		}
	}()

	// subscribe to messages from the device
	deviceTopic := model.GetDeviceSubject(params.TenantID, params.Device.ID)
	sessionTopic := model.GetSessionSubject(params.TenantID, params.SessionID)
	msgChan := make(chan *natsio.Msg, channelSize)
	sub, err := h.nats.ChanSubscribe(sessionTopic, msgChan)
	if err != nil {
		responseError = errors.Wrap(err, errFileTransferSubscribing.Error())
		return
	}

	//nolint:errcheck
	defer sub.Unsubscribe()

	if err = h.filetransferHandshake(msgChan, params.SessionID, deviceTopic); err != nil {
		switch err {
		case errFileTransferTimeout:
			errorStatusCode = http.StatusRequestTimeout
		case errFileTransferNotImplemented, errFileTransferDisabled:
			errorStatusCode = http.StatusBadGateway
		}
		responseError = err
		return
	}

	// Inform the device that we're closing the session
	//nolint:errcheck
	defer h.publishControlMessage(params.SessionID, deviceTopic, ws.MessageTypeClose, nil)

	// initialize the file transfer
	req := wsft.UploadRequest{
		SrcPath: request.SrcPath,
		Path:    request.Path,
		UID:     request.UID,
		GID:     request.GID,
		Mode:    request.Mode,
	}
	if err := h.publishFileTransferProtoMessage(params.SessionID,
		params.UserID, deviceTopic, wsft.MessageTypePut, req, 0); err != nil {
		responseError = err
		return
	}

	// receive the message from the device
	select {
	case wsMessage := <-msgChan:
		msg, msgBody, err := h.decodeFileTransferProtoMessage(wsMessage.Data)
		if err != nil {
			responseError = err
			return
		}

		// process incoming messages from the device by type
		switch msg.Header.MsgType {

		// error message, stop here
		case wsft.MessageTypeError:
			errorMsg := msgBody.(*wsft.Error)
			errorStatusCode = http.StatusBadRequest
			responseError = errors.New(*errorMsg.Error)
			return

		// you can continue the upload
		case wsft.MessageTypeACK:
		}

	// no message after timeout expired, stop here
	case <-time.After(fileTransferTimeout):
		errorStatusCode = http.StatusRequestTimeout
		responseError = errFileTransferTimeout
		return
	}

	// receive the ack message from the device
	latestAckOffsets := make(chan int64, 1)
	errorChan := make(chan error)
	go h.uploadFileResponseHandleInboundMessages(
		c, params, msgChan, errorChan, latestAckOffsets,
	)

	h.uploadFileResponseWriter(
		c, params, request, errorChan, latestAckOffsets, &errorStatusCode, &responseError,
	)
}

func (h ManagementController) uploadFileResponseWriter(c *gin.Context,
	params *fileTransferParams, request *model.UploadFileRequest,
	errorChan chan error, latestAckOffsets <-chan int64,
	errorStatusCode *int, responseError *error) {
	var (
		offset          int64
		latestAckOffset int64
	)
	deviceTopic := model.GetDeviceSubject(params.TenantID, params.Device.ID)

	timeout := time.NewTimer(fileTransferTimeout)
	data := make([]byte, fileTransferBufferSize)
	for {
		n, err := request.File.Read(data)
		if err != nil && err != io.EOF {
			if err == io.ErrUnexpectedEOF {
				*errorStatusCode = http.StatusBadRequest
				*responseError = errors.New(
					"malformed request body: " +
						"did not find closing multipart boundary",
				)
			} else {
				*responseError = err
			}
			return
		} else if n == 0 {
			if err := h.publishFileTransferProtoMessage(params.SessionID,
				params.UserID, deviceTopic, wsft.MessageTypeChunk, nil,
				offset); err != nil {
				*responseError = err
				return
			}
			break
		}

		// send the chunk
		if err := h.publishFileTransferProtoMessage(params.SessionID,
			params.UserID, deviceTopic, wsft.MessageTypeChunk, data[0:n],
			offset); err != nil {
			*responseError = err
			return
		}

		// update the offset
		offset += int64(n)

		// wait for acks, in case the ack sliding window is over
		if offset > latestAckOffset+int64(fileTransferBufferSize*ackSlidingWindowRecv) {
			timeout.Reset(fileTransferTimeout)
			select {
			case err := <-errorChan:
				*errorStatusCode = http.StatusBadRequest
				*responseError = err
				return
			case latestAckOffset = <-latestAckOffsets:
			case <-timeout.C:
				*errorStatusCode = http.StatusRequestTimeout
				*responseError = errFileTransferTimeout
				return
			}
		} else {
			// in case of error, report it
			select {
			case err := <-errorChan:
				*errorStatusCode = http.StatusBadRequest
				*responseError = err
				return
			default:
			}
		}

	}

	for offset > latestAckOffset {
		timeout.Reset(fileTransferTimeout)
		select {
		case latestAckOffset = <-latestAckOffsets:
		case <-timeout.C:
			*errorStatusCode = http.StatusRequestTimeout
			*responseError = errFileTransferTimeout
			return
		}
	}

	c.Writer.WriteHeader(http.StatusCreated)
}

func (h ManagementController) parseUploadFileRequest(c *gin.Context) (*model.UploadFileRequest,
	error) {
	reader, err := c.Request.MultipartReader()
	if err != nil {
		return nil, err
	}

	request := &model.UploadFileRequest{}
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		var n int
		data := make([]byte, fileTransferBufferSize)
		partName := part.FormName()
		switch partName {
		case fieldUploadPath, fieldUploadUID, fieldUploadGID, fieldUploadMode:
			n, err = part.Read(data)
			var value string
			if err == nil || err == io.EOF {
				value = string(data[:n])
			}
			switch partName {
			case fieldUploadPath:
				request.Path = &value
			case fieldUploadUID:
				v, err := strconv.Atoi(string(data[:n]))
				if err != nil {
					return nil, err
				}
				nUID := uint32(v)
				request.UID = &nUID
			case fieldUploadGID:
				v, err := strconv.Atoi(string(data[:n]))
				if err != nil {
					return nil, err
				}
				nGID := uint32(v)
				request.GID = &nGID
			case fieldUploadMode:
				v, err := strconv.ParseUint(string(data[:n]), 8, 32)
				if err != nil {
					return nil, err
				}
				nMode := uint32(v)
				request.Mode = &nMode
			}
			part.Close()
		case fieldUploadFile:
			filename := part.FileName()
			request.SrcPath = &filename
			request.File = part
		}
		// file is the last part we can process, in order to avoid loading it in memory
		if request.File != nil {
			break
		}
	}

	return request, nil
}

func (h ManagementController) UploadFile(c *gin.Context) {
	l := log.FromContext(c.Request.Context())

	params, statusCode, err := h.getFileTransferParams(c)
	if err != nil {
		l.Error(err.Error())
		c.JSON(statusCode, gin.H{"error": err.Error()})
		return
	}

	request, err := h.parseUploadFileRequest(c)
	if err != nil {
		l.Error(err.Error())
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := request.Validate(); err != nil {
		l.Error(err.Error())
		c.JSON(http.StatusBadRequest, gin.H{
			"error": errors.Wrap(err, "bad request").Error(),
		})
		return
	}

	defer request.File.Close()

	allowed, err := h.fileTransferAllowed(c, params.TenantID, params.Device.ID)
	if err != nil {
		l.Error(err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": errors.Wrap(err, "failed to check RBAC"),
		})
		return
	} else if !allowed {
		msg := "Access denied (RBAC)."
		l.Warn(msg)
		c.JSON(http.StatusForbidden, gin.H{
			"error": msg,
		})
		return
	}

	ctx := c.Request.Context()
	if err := h.app.UploadFile(ctx, params.UserID, params.Device.ID,
		*request.Path); err != nil {
		l.Error(err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": errors.Wrap(err, "bad request").Error(),
		})
		return
	}

	h.uploadFileResponse(c, params, request)
}

func (h ManagementController) fileTransferAllowed(c *gin.Context, tenantID string,
	deviceID string) (bool, error) {
	if len(c.Request.Header.Get(model.RBACHeaderRemoteTerminalGroups)) == 0 {
		return true, nil
	}
	groups := strings.Split(
		c.Request.Header.Get(model.RBACHeaderRemoteTerminalGroups), ",")
	return h.app.RemoteTerminalAllowed(c, tenantID, deviceID, groups)
}
