package handlers

import "github.com/FauzanParanditha/portfolio-backend/internal/models"

type TagCreateRequest struct {
	Name string `json:"name" validate:"required"`
	Type string `json:"type" validate:"required"`
}

type TagUpdateRequest = TagCreateRequest

type TagResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

func tagToResponse(t models.Tag) TagResponse {
	return TagResponse{
		ID:   t.ID.String(),
		Name: t.Name,
		Type: t.Type,
	}
}

