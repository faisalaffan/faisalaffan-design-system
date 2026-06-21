package service

import (
	"context"
	"fmt"
	"log"

	"github.com/faisalaffan/faisalaffan-design-system/url-shortener/shortcode"
	"github.com/faisalaffan/faisalaffan-design-system/url-shortener/storage"
)

type ShortenService struct {
	store   storage.Storage
	baseURL string
}

func NewShortenService(store storage.Storage, baseURL string) *ShortenService {
	return &ShortenService{store: store, baseURL: baseURL}
}

type ShortenResult struct {
	ShortURL string `json:"short_url"`
	Code     string `json:"code"`
}

func (s *ShortenService) Shorten(ctx context.Context, longURL string) (*ShortenResult, error) {
	for attempts := 0; attempts < 5; attempts++ {
		code, err := shortcode.Generate()
		if err != nil {
			return nil, fmt.Errorf("generate shortcode: %w", err)
		}

		exists, err := s.store.Exists(ctx, code)
		if err != nil {
			return nil, fmt.Errorf("check exists: %w", err)
		}
		if exists {
			log.Printf("collision: %s, retrying", code)
			continue
		}

		if err := s.store.Save(ctx, code, longURL); err != nil {
			return nil, fmt.Errorf("save: %w", err)
		}

		return &ShortenResult{
			ShortURL: s.baseURL + "/" + code,
			Code:     code,
		}, nil
	}

	return nil, fmt.Errorf("failed to generate unique code after 5 attempts")
}

func (s *ShortenService) Lookup(ctx context.Context, code string) (string, error) {
	return s.store.Get(ctx, code)
}
