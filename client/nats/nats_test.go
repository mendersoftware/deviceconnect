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
	"testing"
	"time"

	"github.com/mendersoftware/go-lib-micro/config"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"

	dconfig "github.com/mendersoftware/deviceconnect/config"
)

func TestConnect(t *testing.T) {
	flag.Parse()
	if testing.Short() {
		t.Skip()
	}

	client := NewClient()
	assert.NotNil(t, client)

	ctx := context.Background()
	err := client.Connect(ctx, "nats://127.0.0.1:9999")
	assert.Error(t, err)

	natsURI := config.Config.GetString(dconfig.SettingNatsURI)
	err = client.Connect(ctx, natsURI)
	assert.NoError(t, err)
}

func TestSubscribePublish(t *testing.T) {
	flag.Parse()
	if testing.Short() {
		t.Skip()
	}

	client := NewClient()
	assert.NotNil(t, client)

	natsURI := config.Config.GetString(dconfig.SettingNatsURI)

	ctx := context.Background()
	err := client.Connect(ctx, natsURI)
	assert.NoError(t, err)

	var recv *nats.Msg
	const subject = "deviceconnect-test"
	client.Subscribe(subject, func(msg *nats.Msg) {
		recv = msg
	})

	msg := []byte("message")
	client.Publish(subject, msg)

	for i := 0; i < 10; i++ {
		time.Sleep(100 * time.Millisecond)
		if recv != nil {
			break
		}
	}

	assert.NotNil(t, recv)
	assert.Equal(t, msg, recv.Data)
}
