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

package clientnats

import (
	"context"
	"flag"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
)

var natsPort int32 = 14420

func setupTestServer() *server.Server {
	port := atomic.AddInt32(&natsPort, 1)
	opts := &server.Options{
		Port: int(port),
	}
	srv, err := server.NewServer(opts)
	if err != nil {
		panic(err)
	}
	go srv.Start()
	// Wait for a second to go routine to setup listener
	for i := 0; srv.Addr() == nil && i < 1000; i++ {
		time.Sleep(time.Millisecond)
	}
	if srv.Addr() == nil {
		panic("failed to setup NATS test server")
	}
	return srv
}

func TestConnect(t *testing.T) {
	t.Parallel()
	flag.Parse()
	if testing.Short() {
		t.Skip()
	}
	srv := setupTestServer()
	defer srv.Shutdown()

	client := NewClient()
	assert.NotNil(t, client)

	ctx := context.Background()

	err := client.Connect(ctx, "nats://localhost:1439")
	assert.Error(t, err)

	err = client.Connect(ctx, srv.Addr().String())
	assert.NoError(t, err)
}

func TestSubscribePublish(t *testing.T) {
	t.Parallel()
	flag.Parse()
	if testing.Short() {
		t.Skip()
	}

	client := NewClient()
	assert.NotNil(t, client)

	srv := setupTestServer()
	defer srv.Shutdown()

	ctx := context.Background()
	err := client.Connect(ctx, srv.Addr().String())
	assert.NoError(t, err)

	var recv *nats.Msg
	const subject = "deviceconnect-test"
	err = client.Subscribe(subject, func(msg *nats.Msg) {
		recv = msg
	})
	assert.NoError(t, err)

	msg := []byte("message")
	err = client.Publish(subject, msg)
	assert.NoError(t, err)

	for i := 0; i < 10; i++ {
		time.Sleep(100 * time.Millisecond)
		if recv != nil {
			break
		}
	}

	assert.NotNil(t, recv)
	assert.Equal(t, msg, recv.Data)
}
