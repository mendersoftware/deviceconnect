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
	"strconv"
	"sync/atomic"
	"time"

	natsio "github.com/nats-io/nats.go"
	"github.com/pkg/errors"
)

const (
	subjectACK = "ack"
)

var ackWait = time.Second * 5

var (
	ErrTimeout = errors.New("nats: timeout waiting for ack")
)

// Client is the nats client
//go:generate ../../utils/mockgen.sh
type Client interface {
	Publish(context.Context, string, []byte) error
	PublishNoAck(string, []byte) error
	ChanSubscribe(string, chan *natsio.Msg) (*natsio.Subscription, error)
}

// NewClient returns a new workflows client
func NewClient(url string, opts ...natsio.Option) (Client, error) {
	natsClient, err := natsio.Connect(url, opts...)
	if err != nil {
		return nil, err
	}
	cid, err := natsClient.GetClientID()
	if err != nil {
		return nil, err
	}
	return &client{
		nc:       natsClient,
		clientID: cid,
	}, nil
}

type client struct {
	nc *natsio.Conn
	// clientID and ackNum is used to create a unique suffix for ack replies
	clientID uint64
	ackNum   uint64
}

func (c *client) Publish(ctx context.Context, subj string, data []byte) error {
	m := natsio.NewMsg(subj)
	m.Data = data
	acknum := atomic.AddUint64(&c.ackNum, 1)
	m.Reply = subj + "." + subjectACK + "." +
		strconv.FormatUint(c.clientID, 16) + "." +
		strconv.FormatUint(acknum, 16)
	ch := make(chan *natsio.Msg, 1)
	sub, err := c.nc.ChanSubscribe(m.Reply, ch)
	if err != nil {
		return err
	}
	defer sub.Unsubscribe() // nolint:errcheck
	err = sub.AutoUnsubscribe(1)
	if err != nil {
		return err
	}
	err = c.nc.PublishMsg(m)
	if err != nil {
		return err
	}
	// NOTE: Only create timer if we need to block
	select {
	case <-ch:
	case <-ctx.Done():
		return ctx.Err()
	default:
		select {
		case <-ch:
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(ackWait):
			return ErrTimeout
		}
	}
	return nil
}

func (c *client) PublishNoAck(subj string, data []byte) error {
	return c.nc.Publish(subj, data)
}

func (c *client) ChanSubscribe(subj string, ch chan *natsio.Msg) (*natsio.Subscription, error) {
	return c.nc.ChanSubscribe(subj, ch)
}
