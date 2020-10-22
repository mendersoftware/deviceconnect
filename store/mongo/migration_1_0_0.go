// Copyright 2020 Northern.tech AS
//
//    All Rights Reserved

package mongo

import (
	"context"

	"github.com/mendersoftware/go-lib-micro/mongo/migrate"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	mopts "go.mongodb.org/mongo-driver/mongo/options"
)

type migration1_0_0 struct {
	client *mongo.Client
	db     string
}

// Up creates the jobs capped collection
func (m *migration1_0_0) Up(from migrate.Version) error {
	ctx := context.Background()
	database := m.client.Database(m.db)

	collSessions := database.Collection(SessionsCollectionName)
	idxSessions := collSessions.Indexes()

	// unique index devices.websocket_id
	indexOptions := mopts.Index()
	// staticcheck will complain on (mongo 4.2) deprecation warning in docs
	indexOptions.SetBackground(false) //nolint:staticcheck
	indexOptions.SetName("user_id_device_id")
	indexOptions.SetUnique(true)
	userDeviceIDIndex := mongo.IndexModel{
		Keys: bson.D{
			{Key: "user_id", Value: 1},
			{Key: "device_id", Value: 1},
		},
		Options: indexOptions,
	}
	if _, err := idxSessions.CreateOne(ctx, userDeviceIDIndex); err != nil {
		return err
	}

	return nil
}

func (m *migration1_0_0) Version() migrate.Version {
	return migrate.MakeVersion(1, 0, 0)
}
