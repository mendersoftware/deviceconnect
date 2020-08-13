// Copyright 2020 Northern.tech AS
//
//    All Rights Reserved

package mongo

import (
	"github.com/mendersoftware/go-lib-micro/mongo/migrate"
	"go.mongodb.org/mongo-driver/mongo"
)

type migration1_0_0 struct {
	client *mongo.Client
	db     string
}

// Up creates the jobs capped collection
func (m *migration1_0_0) Up(from migrate.Version) error {
	return nil
}

func (m *migration1_0_0) Version() migrate.Version {
	return migrate.MakeVersion(1, 0, 0)
}
