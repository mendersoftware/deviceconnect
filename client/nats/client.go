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
	natsio "github.com/nats-io/nats.go"
)

// Client is the nats client
//go:generate ../../utils/mockgen.sh
type Client interface {
	Publish(string, []byte) error
	ChanSubscribe(string, chan *natsio.Msg) (*natsio.Subscription, error)
}

// NewClient returns a new workflows client
func NewClient(url string, opts ...natsio.Option) (Client, error) {
	natsClient, err := natsio.Connect(url, opts...)
	if err != nil {
		return nil, err
	}
	return &client{
		nats: natsClient,
	}, nil
}

type client struct {
	nats *natsio.Conn
}

func (c *client) Publish(subj string, data []byte) error {
	return c.nats.Publish(subj, data)
}

func (c *client) ChanSubscribe(subj string,
	channel chan *natsio.Msg) (*natsio.Subscription, error) {
	return c.nats.ChanSubscribe(subj, channel)
}
