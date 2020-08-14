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

	collDevices := database.Collection(DevicesCollectionName)
	idxDevices := collDevices.Indexes()

	// unique index devices.websocket_id
	indexOptions := mopts.Index()
	indexOptions.SetBackground(false)
	indexOptions.SetName("websocket_id")
	indexOptions.SetUnique(true)
	deviceIDIndex := mongo.IndexModel{
		Keys:    bson.D{{Key: "websocket_id", Value: 1}},
		Options: indexOptions,
	}
	if _, err := idxDevices.CreateOne(ctx, deviceIDIndex); err != nil {
		return err
	}

	return nil
}

func (m *migration1_0_0) Version() migrate.Version {
	return migrate.MakeVersion(1, 0, 0)
}
