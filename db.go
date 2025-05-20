package database

import (
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func Connect(dsn string, logQueries bool) (*gorm.DB, error) {
	// TODO: Check settings
	// TODO: Make singleton patternt form this to avoid multiple db instances
	// TODO: Reconnect strategy
	cfg := &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	}

	if logQueries {
		cfg = &gorm.Config{
			Logger: logger.Default.LogMode(logger.Info),
		}
	}

	instance, err := gorm.Open(postgres.Open(dsn), cfg)

	if err != nil {
		return nil, err
	}

	db, err := instance.DB()

	if err != nil {
		return nil, err
	}

	db.SetConnMaxLifetime(time.Minute * 5)

	return instance, nil
}
