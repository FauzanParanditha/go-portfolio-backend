package handlers

import "github.com/FauzanParanditha/portfolio-backend/internal/models"

// Request body untuk create/update project (dipakai AdminProjectHandler)
type ProjectCreateRequest struct {
	Title         string   `json:"title" validate:"required"`
	Slug          string   `json:"slug" validate:"required"`
	ShortDesc     string   `json:"shortDesc" validate:"required"`
	CoverImageURL string   `json:"coverImageUrl" validate:"required"`
	LiveURL       *string  `json:"liveUrl" validate:"omitempty,url"`
	SourceURL     *string  `json:"sourceUrl" validate:"omitempty,url"`
	IsFeatured    bool     `json:"isFeatured"`
	SortOrder     int      `json:"sortOrder"`
	TagIDs        []string `json:"tagIds"`   // list UUID string
	Features      []string `json:"features"` // list text bullet
}

type ProjectFeatureResponse struct {
	Text string `json:"text"`
}

type ProjectResponse struct {
	ID            string                   `json:"id"`
	Title         string                   `json:"title"`
	Slug          string                   `json:"slug"`
	ShortDesc     string                   `json:"shortDesc"`
	CoverImageURL string                   `json:"coverImageUrl"`
	LiveURL       *string                  `json:"liveUrl,omitempty"`
	SourceURL     *string                  `json:"sourceUrl,omitempty"`
	IsFeatured    bool                     `json:"isFeatured"`
	Tags          []TagResponse            `json:"tags"`
	Features      []ProjectFeatureResponse `json:"features"`
}

// Mapper dari models.Project ke ProjectResponse
func projectToResponse(p models.Project) ProjectResponse {
	tags := make([]TagResponse, 0, len(p.Tags))
	for _, t := range p.Tags {
		tags = append(tags, TagResponse{
			Name: t.Name,
			Type: t.Type,
		})
	}

	features := make([]ProjectFeatureResponse, 0, len(p.Features))
	for _, f := range p.Features {
		features = append(features, ProjectFeatureResponse{
			Text: f.Text,
		})
	}

	return ProjectResponse{
		ID:            p.ID.String(),
		Title:         p.Title,
		Slug:          p.Slug,
		ShortDesc:     p.ShortDesc,
		CoverImageURL: p.CoverImageURL,
		LiveURL:       p.LiveURL,
		SourceURL:     p.SourceURL,
		IsFeatured:    p.IsFeatured,
		Tags:          tags,
		Features:      features,
	}
}

type ProjectsListResponse struct {
	Data []ProjectResponse `json:"data"`
	Meta interface{}       `json:"meta"`
}

type ErrorResponse struct {
	Error struct {
		Message string            `json:"message"`
		Code    string            `json:"code"`
		Details map[string]string `json:"details,omitempty"`
	} `json:"error"`
	RequestID string `json:"requestId,omitempty"`
}
