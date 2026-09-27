package repositories

import "testing"

func TestNewStorageUnavailableDatabase(t *testing.T) {
	storage, err := NewStorage("postgres://auth:secret@127.0.0.1:1/auth?sslmode=disable&connect_timeout=1")
	if err == nil {
		if storage != nil {
			_ = storage.Close()
		}
		t.Fatal("NewStorage with unavailable database succeeded")
	}
	if storage != nil {
		t.Fatalf("storage=%v want nil", storage)
	}
}

func TestNewStorageInvalidDSN(t *testing.T) {
	storage, err := NewStorage("postgres://%")
	if err == nil {
		if storage != nil {
			_ = storage.Close()
		}
		t.Fatal("NewStorage with invalid DSN succeeded")
	}
	if storage != nil {
		t.Fatalf("storage=%v want nil", storage)
	}
}
