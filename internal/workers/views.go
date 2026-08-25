package workers

import (
	"blog/internal/repositories"
	"context"
	"time"

	"github.com/google/uuid"
)

func updateViews(s *repositories.Storage) error {
	viewsToUpdate, err := s.GetAndClearViewsCount()

	if err != nil {
		return err
	}

	for articleIDStr, viewsCount := range viewsToUpdate {
		articleID, err := uuid.Parse(articleIDStr)
		if err != nil {
			continue
		}

		err = s.UpdateArticleViews(viewsCount, articleID)
		if err != nil {
			continue
		}
	}

	return nil
}

func StartViewsUpdaterWorker(ctx context.Context, s *repositories.Storage) error {
	ticker := time.NewTicker(3 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			updateViews(s)
		case <-ctx.Done():
			return nil
		}
	}
}
