package repositories

import (
	"context"
	"database/sql"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/redis/go-redis/v9"
)

type Storage struct {
	db    *sql.DB
	redis *redis.Client
}

func initPostgres(pgDSN string) (*sql.DB, error) {
	db, err := sql.Open("pgx", pgDSN)
	if err != nil {
		return nil, err
	}

	if err := db.Ping(); err != nil {
		return nil, err
	}

	return db, nil
}

func initRedis(redisDSN string) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     redisDSN,
		Password: "",
		DB:       0,
	})

	if err := client.Ping(context.Background()).Err(); err != nil {
		return nil, err
	}

	return client, nil
}

func NewStorage(pgDSN string, redisDSN string) (*Storage, error) {
	db, err := initPostgres(pgDSN)
	if err != nil {
		return nil, err
	}

	redis, err := initRedis(redisDSN)
	if err != nil {
		return nil, err
	}

	return &Storage{
		db:    db,
		redis: redis,
	}, nil
}

func (s *Storage) Close() error {
	if err := s.redis.Close(); err != nil {
		return err
	}

	if err := s.db.Close(); err != nil {
		return err
	}

	return nil
}
