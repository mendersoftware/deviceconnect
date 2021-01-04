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

package workflows

import (
	"time"

	"github.com/go-ozzo/ozzo-validation/v4"
	"github.com/go-ozzo/ozzo-validation/v4/is"
)

type AuditWorkflow struct {
	RequestID string   `json:"request_id"`
	TenantID  string   `json:"tenant_id"`
	AuditLog  AuditLog `json:"auditlog"`
}

type Action string

const (
	ActionCreate Action = "create"
	ActionDelete Action = "delete"
	ActionUpdate Action = "update"
)

type ActorType string

const (
	ActorDevice ActorType = "device"
	ActorUser   ActorType = "user"
)

type Actor struct {
	ID             string    `json:"id"`
	Type           ActorType `json:"type"`
	Email          string    `json:"email,omitempty"`
	DeviceIdentity string    `json:"identity_data,omitempty"`
}

func (a Actor) Validate() error {
	err := validation.ValidateStruct(&a,
		validation.Field(&a.ID, validation.Required),
		validation.Field(&a.Type,
			validation.In(ActorUser, ActorDevice),
			validation.Required,
		),
	)
	if err != nil {
		return err
	}

	switch a.Type {
	case ActorUser:
		err = validation.ValidateStruct(&a,
			validation.Field(&a.Email, is.EmailFormat),
			validation.Field(&a.DeviceIdentity, validation.Empty),
		)
	case ActorDevice:
		err = validation.ValidateStruct(&a,
			validation.Field(&a.DeviceIdentity, is.JSON),
			validation.Field(&a.Email, validation.Empty),
		)
	}
	return err
}

type Terminal struct {
	DeviceID string `json:"device_id"`
}

func (t Terminal) Validate() error {
	return validation.ValidateStruct(&t,
		validation.Field(&t.DeviceID, validation.Required),
	)
}

type ObjectType string

const ObjectTerminal ObjectType = "terminal"

type Object struct {
	ID   string     `json:"id"`
	Type ObjectType `json:"type"`

	Terminal *Terminal `json:"terminal,omitempty"`
}

func (o Object) Validate() error {
	err := validation.ValidateStruct(&o,
		validation.Field(&o.ID, validation.Required),
		validation.Field(&o.Type,
			validation.Required,
			validation.In(ObjectTerminal),
		),
		validation.Field(&o.Terminal, validation.Required),
	)
	return err
}

type AuditLog struct {
	Action  Action    `json:"action"`
	Actor   Actor     `json:"actor"`
	Object  Object    `json:"object"`
	Change  string    `json:"change,omitempty"`
	EventTS time.Time `json:"time,omitempty"`
}

func (l AuditLog) Validate() error {
	return validation.ValidateStruct(&l,
		validation.Field(&l.Actor, validation.Required),
		validation.Field(&l.Action, validation.In(
			ActionCreate, ActionUpdate, ActionDelete,
		), validation.Required),
		validation.Field(&l.Object, validation.Required),
		validation.Field(&l.EventTS, validation.Required),
	)
}
