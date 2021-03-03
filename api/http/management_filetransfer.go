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
	"context"
	"encoding/json"
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
)

var fileTransferPingInterval = 30 * time.Second
var fileTransferTimeout = 60 * time.Second
var fileTransferBufferSize = 4096
var ackSlidingWindowSend = 10
var ackSlidingWindowRecv = 20

var (
	errFileTranserMarshalling   = errors.New("failed to marshal the request")
	errFileTranserUnmarshalling = errors.New("failed to unmarshal the request")
	errFileTranserPublishing    = errors.New("failed to publish the message")
	errFileTranserSubscribing   = errors.New("failed to subscribe to the mesages")
	errFileTranserTimeout       = errors.New("file transfer timed out")
	errFileTranserFailed        = errors.New("file transfer failed")
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

	if c.Request.Body == nil {
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
			return errors.Wrap(err, errFileTranserMarshalling.Error())
		}
	}

	msg := &ws.ProtoMsg{
		Header: ws.ProtoHdr{
			Proto:     ws.ProtoTypeFileTransfer,
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
		return errors.Wrap(err, errFileTranserMarshalling.Error())
	}

	err = h.nats.Publish(deviceTopic, data)
	if err != nil {
		return errors.Wrap(err, errFileTranserPublishing.Error())
	}
	return nil
}

func (h ManagementController) publishFileTransferPing(sessionID, deviceTopic string) {
	msg := &ws.ProtoMsg{
		Header: ws.ProtoHdr{
			Proto:     ws.ProtoTypeControl,
			MsgType:   ws.MessageTypePing,
			SessionID: sessionID,
		},
	}
	data, err := msgpack.Marshal(msg)
	if err == nil {
		_ = h.nats.Publish(deviceTopic, data)
	}
}

func (h ManagementController) decodeFileTransferProtoMessage(data []byte) (*ws.ProtoMsg,
	interface{}, error) {
	msg := &ws.ProtoMsg{}
	err := msgpack.Unmarshal(data, msg)
	if err != nil {
		return nil, nil, errors.Wrap(err, errFileTranserUnmarshalling.Error())
	}

	switch msg.Header.MsgType {
	case wsft.MessageTypeError:
		msgBody := &wsft.Error{}
		err := msgpack.Unmarshal(msg.Body, msgBody)
		if err != nil {
			return nil, nil, errors.Wrap(err, errFileTranserUnmarshalling.Error())
		}
		return msg, msgBody, nil
	case wsft.MessageTypeFileInfo:
		msgBody := &wsft.FileInfo{}
		err := msgpack.Unmarshal(msg.Body, msgBody)
		if err != nil {
			return nil, nil, errors.Wrap(err, errFileTranserUnmarshalling.Error())
		}
		return msg, msgBody, nil
	}

	return msg, nil, nil
}

func writeHeaders(c *gin.Context, fileInfo *wsft.FileInfo) {
	c.Writer.WriteHeader(http.StatusOK)
	c.Writer.Header().Add(hdrContentType, "application/octet-stream")
	if fileInfo.Path != nil {
		filename := path.Base(*fileInfo.Path)
		c.Writer.Header().Add(hdrContentDisposition, "attachment; filname=\""+filename+"\"")
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
	if !*responseHeaderSent && *responseError != nil {
		log.FromContext(c).Error((*responseError).Error())
		status := http.StatusInternalServerError
		// errFileTranserFailed is a special case, we return 400 instead of 500
		if strings.Contains((*responseError).Error(), errFileTranserFailed.Error()) {
			status = http.StatusBadRequest
		} else if *responseError == errFileTranserTimeout {
			status = http.StatusRequestTimeout
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
		responseError = errors.Wrap(err, errFileTranserSubscribing.Error())
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

	ticker := time.NewTicker(fileTransferPingInterval)
	defer ticker.Stop()

	// handle messages from the device
	latestMessage := time.Now()
	latestOffset := int64(0)
	numberOfChunks := 0
	for {
		ctx, cancel := context.WithDeadline(context.Background(),
			latestMessage.Add(fileTransferTimeout))
		defer cancel()

		select {
		case wsMessage := <-msgChan:
			latestMessage = time.Now()
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
				if *errorMsg.MessageType == wsft.MessageTypeStat {
					responseError = errors.Wrap(errors.New(*errorMsg.Error),
						errFileTranserFailed.Error())
				} else {
					responseError = errors.New(*errorMsg.Error)
				}
				return

			// file stat response, if okay, let's get the file
			case wsft.MessageTypeFileInfo:
				req := wsft.GetFile{
					Path: request.Path,
				}
				if err := h.publishFileTransferProtoMessage(params.SessionID,
					params.UserID, deviceTopic, wsft.MessageTypeGet,
					req, 0); err != nil {
					responseError = err
					return
				}

				fileInfo := msgBody.(*wsft.FileInfo)
				writeHeaders(c, fileInfo)
				responseHeaderSent = true

			// file data chunk
			case wsft.MessageTypeChunk:
				if msg.Body == nil {
					if err := h.publishFileTransferProtoMessage(
						params.SessionID, params.UserID, deviceTopic,
						wsft.MessageTypeACK, nil,
						latestOffset); err != nil {
						return
					}
					return
				}

				// verify the offset property
				propOffset, _ := msg.Header.Properties[PropertyOffset].(int64)
				if propOffset != latestOffset {
					responseError = errors.Wrap(errFileTranserFailed,
						"wrong offset received")
					return
				}
				latestOffset += int64(len(msg.Body))

				_, err := c.Writer.Write(msg.Body)
				if err != nil {
					return
				}

				numberOfChunks++
				if numberOfChunks >= ackSlidingWindowSend {
					if err := h.publishFileTransferProtoMessage(
						params.SessionID, params.UserID, deviceTopic,
						wsft.MessageTypeACK, nil,
						latestOffset); err != nil {
						return
					}
					numberOfChunks = 0
				}
			}

		// send a Ping message to keep the session alive
		case <-ticker.C:
			h.publishFileTransferPing(params.SessionID, deviceTopic)

		// no message after timeout expired, stop here
		case <-ctx.Done():
			responseError = errFileTranserTimeout
			return
		}
	}
}

func (h ManagementController) DownloadFile(c *gin.Context) {
	params, statusCode, err := h.getFileTransferParams(c)
	if err != nil {
		c.JSON(statusCode, gin.H{"error": err.Error()})
		return
	}

	rawData, err := c.GetRawData()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "failed to get the request body",
		})
		return
	}

	request := &model.DownloadFileRequest{}
	if err = json.Unmarshal(rawData, request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": errors.Wrap(err, "invalid request body").Error(),
		})
		return
	}

	if err := request.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": errors.Wrap(err, "bad request").Error(),
		})
		return
	}

	allowed, err := h.fileTransferAllowed(c, params.TenantID, params.Device.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": errors.Wrap(err, "failed to check RBAC"),
		})
		return
	} else if !allowed {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "Access denied (RBAC).",
		})
		return
	}

	if err := h.app.DownloadFile(c, params.UserID, params.Device.ID,
		*request.Path); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": errors.Wrap(err, "bad request").Error(),
		})
		return
	}

	h.downloadFileResponse(c, params, request)
}

func (h ManagementController) uploadFileResponseHandleInboundMessages(c *gin.Context,
	msgChan chan *natsio.Msg, errorChan chan error, latestAckOffset *int64,
	latestAckOffsets chan int64) {
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
				if propOffset > *latestAckOffset {
					*latestAckOffset = propOffset
					select {
					case latestAckOffsets <- *latestAckOffset:
					default:
					}
				}
			}
		case <-c.Done():
			return
		}
	}
}

func (h ManagementController) uploadFileResponse(c *gin.Context, params *fileTransferParams,
	request *model.UploadFileRequest) {
	// send a JSON-encoded error message in case of failure
	var responseError error
	errorStatusCode := http.StatusInternalServerError
	defer func() {
		if responseError != nil {
			log.FromContext(c).Error(responseError.Error())
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
		responseError = errors.Wrap(err, errFileTranserSubscribing.Error())
		return
	}

	//nolint:errcheck
	defer sub.Unsubscribe()

	// initialize the file transfer
	req := wsft.FileInfo{
		Path: request.Path,
		UID:  request.UID,
		GID:  request.GID,
		Mode: request.Mode,
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
		responseError = errFileTranserTimeout
		return
	}

	// receive the ack message from the device
	latestAckOffset := int64(-1)
	latestAckOffsets := make(chan int64, 1)
	errorChan := make(chan error)
	go h.uploadFileResponseHandleInboundMessages(c, msgChan, errorChan, &latestAckOffset,
		latestAckOffsets)

	h.uploadFileResponseWriter(c, params, request, errorChan, &latestAckOffset,
		latestAckOffsets, &errorStatusCode, &responseError)
}

func (h ManagementController) uploadFileResponseWriter(c *gin.Context,
	params *fileTransferParams, request *model.UploadFileRequest,
	errorChan chan error, latestAckOffset *int64, latestAckOffsets chan int64,
	errorStatusCode *int, responseError *error) {
	deviceTopic := model.GetDeviceSubject(params.TenantID, params.Device.ID)

	data := make([]byte, fileTransferBufferSize)
	offset := int64(0)
	for {
		n, err := request.File.Read(data)
		if err != nil && err != io.EOF {
			*responseError = err
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

		// drain latestAckOffsets and errorChan
		select {
		case <-latestAckOffsets:
		default:
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
		if offset > *latestAckOffset+int64(fileTransferBufferSize*ackSlidingWindowRecv) {
			select {
			case err := <-errorChan:
				*errorStatusCode = http.StatusBadRequest
				*responseError = err
				return
			case <-latestAckOffsets:
			case <-time.After(fileTransferTimeout):
				*errorStatusCode = http.StatusRequestTimeout
				*responseError = errFileTranserTimeout
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
				v, err := strconv.Atoi(string(data[:n]))
				if err != nil {
					return nil, err
				}
				nMode := uint32(v)
				request.Mode = &nMode
			}
			part.Close()
		case fieldUploadFile:
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
	params, statusCode, err := h.getFileTransferParams(c)
	if err != nil {
		c.JSON(statusCode, gin.H{"error": err.Error()})
		return
	}

	request, err := h.parseUploadFileRequest(c)
	if err != nil {
		c.JSON(statusCode, gin.H{"error": err.Error()})
		return
	}

	if err := request.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": errors.Wrap(err, "bad request").Error(),
		})
		return
	}

	defer request.File.Close()

	allowed, err := h.fileTransferAllowed(c, params.TenantID, params.Device.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": errors.Wrap(err, "failed to check RBAC"),
		})
		return
	} else if !allowed {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "Access denied (RBAC).",
		})
		return
	}

	if err := h.app.UploadFile(c, params.UserID, params.Device.ID,
		*request.Path); err != nil {
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
