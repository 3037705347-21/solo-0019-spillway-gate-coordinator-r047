package coordination

import (
	"time"

	"example.com/spillway-gate-coordinator/internal/storage"
)

type Service struct {
	repository storage.Repository
	now        func() time.Time
	newID      func(prefix string) (string, error)
	newToken   func() (string, error)
}

func NewService(repository storage.Repository) *Service {
	return &Service{
		repository: repository,
		now:        time.Now,
		newID:      randomIdentifier,
		newToken:   randomToken,
	}
}
