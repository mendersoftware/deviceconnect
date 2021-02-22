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

package mongo

import (
	"reflect"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/bsoncodec"
	"go.mongodb.org/mongo-driver/bson/bsonrw"
	"go.mongodb.org/mongo-driver/bson/bsontype"
)

var (
	tUUID = reflect.TypeOf(uuid.UUID{})
)

func init() {
	// Add UUID encoder/decoder for github.com/google/uuid.UUID
	bson.DefaultRegistry = bson.NewRegistryBuilder().
		RegisterTypeEncoder(tUUID, bsoncodec.ValueEncoderFunc(uuidEncodeValue)).
		RegisterTypeDecoder(tUUID, bsoncodec.ValueDecoderFunc(uuidDecodeValue)).
		Build()
}

func uuidEncodeValue(ec bsoncodec.EncodeContext, w bsonrw.ValueWriter, val reflect.Value) error {
	if !val.IsValid() || val.Type() != tUUID {
		return bsoncodec.ValueEncoderError{
			Name:     "UUIDEncodeValue",
			Types:    []reflect.Type{tUUID},
			Received: val,
		}
	}
	uid := val.Interface().(uuid.UUID)
	return w.WriteBinaryWithSubtype(uid[:], bsontype.BinaryUUID)
}

func uuidDecodeValue(ec bsoncodec.DecodeContext, r bsonrw.ValueReader, val reflect.Value) error {
	if !val.CanSet() || val.Type() != tUUID {
		return bsoncodec.ValueDecoderError{
			Name:     "UUIDDecodeValue",
			Types:    []reflect.Type{tUUID},
			Received: val,
		}
	}

	var (
		data    []byte
		err     error
		subtype byte
		uid     uuid.UUID = uuid.Nil
	)
	switch rType := r.Type(); rType {
	case bsontype.Binary:
		data, subtype, err = r.ReadBinary()
		switch subtype {
		case bsontype.BinaryGeneric:
			if len(data) != 16 {
				return errors.Errorf(
					"cannot decode %v as a UUID: "+
						"incorrect length: %d",
					data, len(data),
				)
			}

			fallthrough
		case bsontype.BinaryUUID, bsontype.BinaryUUIDOld:
			copy(uid[:], data)

		default:
			err = errors.Errorf(
				"cannot decode %v as a UUID: "+
					"incorrect subtype 0x%02x",
				data, subtype,
			)
		}

	case bsontype.Undefined:
		err = r.ReadUndefined()

	case bsontype.Null:
		err = r.ReadNull()

	default:
		err = errors.Errorf("cannot decode %v as a UUID", rType)
	}

	if err != nil {
		return err
	}
	val.Set(reflect.ValueOf(uid))
	return nil
}
