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
	"context"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	mopts "go.mongodb.org/mongo-driver/mongo/options"

	"github.com/mendersoftware/go-lib-micro/identity"
	"github.com/mendersoftware/go-lib-micro/mongo/migrate"
	mstorev1 "github.com/mendersoftware/go-lib-micro/store"
	mstore "github.com/mendersoftware/go-lib-micro/store/v2"
)

const (
	findBatchSize = 255
)

type migration_2_0_0 struct {
	client *mongo.Client
	db     string
}

func (m *migration_2_0_0) Up(from migrate.Version) error {
	ctx := context.Background()
	client := m.client
	collections := map[string]struct {
		Indexes []mongo.IndexModel
	}{
		DevicesCollectionName: {
			Indexes: []mongo.IndexModel{
				{
					Keys: bson.D{
						{Key: mstore.FieldTenantID, Value: 1},
						{Key: "_id", Value: 1},
					},
					Options: mopts.Index().
						SetName(mstore.FieldTenantID + "_" + "_id"),
				},
			},
		},
		SessionsCollectionName: {
			Indexes: []mongo.IndexModel{
				{
					Keys: bson.D{
						{Key: mstore.FieldTenantID, Value: 1},
						{Key: "device_id", Value: 1},
					},
					Options: mopts.Index().
						SetName(mstore.FieldTenantID + "_" + "device_id"),
				},
			},
		},
	}

	tenantID := mstorev1.TenantFromDbName(m.db, DbName)
	ctx = identity.WithContext(ctx, &identity.Identity{
		Tenant: tenantID,
	})

	for collection := range collections {
		// get all the documents in the collection
		findOptions := mopts.Find().
			SetBatchSize(findBatchSize).
			SetSort(bson.D{{Key: "_id", Value: 1}})
		coll := client.Database(m.db).Collection(collection)
		collOut := client.Database(DbName).Collection(collection)
		_, err := collOut.Indexes().CreateMany(ctx, collections[collection].Indexes)
		if err != nil {
			return err
		}

		cur, err := coll.Find(ctx, bson.D{}, findOptions)
		if err != nil {
			return err
		}
		defer cur.Close(ctx)

		writes := make([]mongo.WriteModel, 0, findBatchSize)

		// migrate the documents
		for cur.Next(ctx) {
			item := bson.D{}
			err := cur.Decode(&item)
			if err != nil {
				return err
			}

			item = mstore.WithTenantID(ctx, item)
			if m.db == DbName {
				filter := bson.D{}
				for _, i := range item {
					if i.Key == "_id" {
						filter = append(filter, i)
					}
				}
				writes = append(writes,
					mongo.NewReplaceOneModel().
						SetFilter(filter).SetReplacement(item))
			} else {
				writes = append(writes, mongo.NewInsertOneModel().SetDocument(item))
			}
			if len(writes) == findBatchSize {
				if err != nil {
					return err
				}
				writes = writes[:0]
			}
		}
		if len(writes) > 0 {
			_, err := collOut.BulkWrite(ctx, writes)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (m *migration_2_0_0) Version() migrate.Version {
	return migrate.MakeVersion(1, 0, 0)
}
