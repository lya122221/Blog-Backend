package models

import "fmt"

type Weights struct {
	Impression int
	Open       int
	Like       int
	Comment    int
}

func DefaultWeights() Weights {
	return Weights{Impression: 1, Open: 3, Like: 6, Comment: 6}
}

func (weights Weights) ScoreDelta(eventType EventType) (int, error) {
	switch eventType {
	case ArticleImpression:
		return weights.Impression, nil
	case ArticleOpened:
		return weights.Open, nil
	case ArticleLiked:
		return weights.Like, nil
	case ArticleUnliked:
		return -weights.Like, nil
	case CommentCreated:
		return weights.Comment, nil
	case ArticleCreated, ArticleUpdated, ArticleDeleted:
		return 0, nil
	default:
		return 0, fmt.Errorf("unsupported event type %q", eventType)
	}
}
