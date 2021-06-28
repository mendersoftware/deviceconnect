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

package nats

import (
	"context"
	"errors"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
)

func init() {
	ackWait = time.Second
}

var natsPort int32 = 42069

func NewNATSTestServer(t *testing.T) (URI string) {
	port := atomic.AddInt32(&natsPort, 1)
	opts := &server.Options{
		Port: int(port),
	}
	srv, err := server.NewServer(opts)
	if err != nil {
		panic(err)
	}
	go srv.Start()
	t.Cleanup(srv.Shutdown)

	// Spinlock until go routine is listening
	for i := 0; srv.Addr() == nil && i < 1000; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	if srv.Addr() == nil {
		panic("failed to setup NATS test server")
	}
	uri, err := url.Parse("nats://" + srv.Addr().String())
	if err != nil {
		panic(err)
	}

	return uri.String()
}

func TestPublishSubscribe(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		Name string

		NoAck    bool
		CTX      context.Context
		URI      string
		SubTopic string
		OnRecv   func(t *testing.T, ch chan *nats.Msg)

		ClientError error
		SubError    error
		PubError    error
	}{{
		Name: "ok",

		CTX:      context.Background(),
		SubTopic: "foo.bar",
		OnRecv: func(t *testing.T, ch chan *nats.Msg) {
			m := <-ch
			m.Respond(nil)
		},
	}, {
		Name: "ok, no ack",

		CTX:      context.Background(),
		SubTopic: "foo.bar",
		NoAck:    true,
		OnRecv: func(t *testing.T, ch chan *nats.Msg) {
			select {
			case <-ch:
			case <-time.After(time.Second * 5):
				assert.FailNow(t, "timeout waiting for message")
			}
		},
	}, {
		Name: "error invalid URI",

		CTX:         context.Background(),
		URI:         "bats://localhost",
		ClientError: errors.New(""),
	}, {
		Name: "error bad topic",

		SubTopic: ".foo.bar",
		SubError: nats.ErrBadSubject,
	}, {
		Name: "error timeout waiting for ack",

		CTX:      context.Background(),
		SubTopic: "foo.bar",
		OnRecv: func(t *testing.T, ch chan *nats.Msg) {
			<-ch
		},
		PubError: ErrTimeout,
	}, {
		Name: "error context cancelled",

		CTX: func() context.Context {
			ctx, cancel := context.WithCancel(context.TODO())
			cancel()
			return ctx
		}(),
		SubTopic: "foo.bar",
		PubError: context.Canceled,
	}}
	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()
			uri := NewNATSTestServer(t)
			if tc.URI != "" {
				uri = tc.URI
			}
			conn, err := NewClient(uri)
			if tc.ClientError != nil {
				if assert.Error(t, err) {
					assert.Regexp(t, tc.ClientError.Error(), err.Error())
				}
				return
			}
			if !assert.NoError(t, err) {
				return
			}

			ch := make(chan *nats.Msg)
			s, err := conn.ChanSubscribe(tc.SubTopic, ch)
			if err == nil {
				defer s.Unsubscribe()
			}
			if tc.SubError != nil {
				if assert.Error(t, err) {
					assert.Regexp(t, tc.SubError.Error(), err.Error())
				}
				return
			}
			if !assert.NoError(t, err) {
				return
			}
			done := make(chan struct{})
			if tc.OnRecv != nil {
				go func() {
					tc.OnRecv(t, ch)
					close(done)
				}()
			} else {
				close(done)
			}
			if tc.NoAck {
				err = conn.PublishNoAck(tc.SubTopic, nil)
			} else {
				err = conn.Publish(tc.CTX, tc.SubTopic, nil)
			}
			if tc.PubError != nil {
				if assert.Error(t, err) {
					assert.Regexp(t, tc.PubError.Error(), err.Error())
				}
				return
			}
			if !assert.NoError(t, err) {
				return
			}
			<-done
		})
	}
}
