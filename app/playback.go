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

package app

import (
	"github.com/mendersoftware/go-lib-micro/ws"
	"github.com/mendersoftware/go-lib-micro/ws/shell"
	"github.com/nats-io/nats.go"
	"github.com/vmihailenco/msgpack/v5"
	"time"
)

const (
	DefaultPlaybackSleepIntervalMs = uint(100)
)

type Playback struct {
	sessionID         string
	deviceChan        chan *nats.Msg
	sleepMilliseconds uint
}

func NewPlayback(sessionID string, deviceChan chan *nats.Msg, sleepMilliseconds uint) *Playback {
	return &Playback{
		deviceChan:        deviceChan,
		sessionID:         sessionID,
		sleepMilliseconds: sleepMilliseconds,
	}
}

func (r *Playback) Write(d []byte) (n int, err error) {
	msg := ws.ProtoMsg{
		Header: ws.ProtoHdr{
			Proto:     ws.ProtoTypeShell,
			MsgType:   shell.MessageTypeShellCommand,
			SessionID: r.sessionID,
			Properties: map[string]interface{}{
				"status": shell.NormalMessage,
			},
		},
		Body: nil,
	}

	m := nats.Msg{
		Subject: "playback",
		Reply:   "no-reply",
		Data:    nil,
		Sub:     nil,
	}

	msg.Body = d
	data, _ := msgpack.Marshal(msg)
	m.Data = data
	time.Sleep(time.Duration(r.sleepMilliseconds) * time.Millisecond)
	r.deviceChan <- &m
	return len(d), nil
}
