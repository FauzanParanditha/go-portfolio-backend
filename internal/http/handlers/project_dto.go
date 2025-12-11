package handlers

import (
	"github.com/FauzanParanditha/portfolio-backend/internal/models"
)

// Request body untuk create/update project (dipakai AdminProjectHandler)
type ProjectCreateRequest struct {
	Title         string `json:"title" validate:"required"`
	Slug          string `json:"slug" validate:"required"`
	ShortDesc     string `json:"shortDesc" validate:"required"`
	LongDesc      string `json:"longDescription"` // boleh kosong, tapi idealnya diisi
	CoverImageURL string `json:"coverImageUrl" validate:"required"`

	Category string `json:"category"`
	Timeline string `json:"timeline"`
	Role     string `json:"role"`

	Challenge string `json:"challenge"`
	Solution  string `json:"solution"`

	Results          []string       `json:"results"`          // array bullet hasil
	TechnicalDetails map[string]any `json:"technicalDetails"` // JSON object

	DemoURL *string `json:"demoUrl" validate:"omitempty,url"`
	RepoURL *string `json:"repoUrl" validate:"omitempty,url"`

	Screenshots []string `json:"screenshots"` // list URL gambar, sederhana dulu

	IsFeatured bool     `json:"isFeatured"`
	SortOrder  int      `json:"sortOrder"`
	TagIDs     []string `json:"tagIds"`   // list UUID string
	Features   []string `json:"features"` // list text bullet
}

// Response kecil untuk feature
type ProjectFeatureResponse struct {
	Text string `json:"text"`
}

// Response kecil untuk screenshot
type ProjectScreenshotResponse struct {
	ImageURL  string `json:"imageUrl"`
	SortOrder int    `json:"sortOrder"`
}

// Response utama untuk Project (public + admin)
type ProjectResponse struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	Slug          string `json:"slug"`
	ShortDesc     string `json:"shortDesc"`
	LongDesc      string `json:"longDescription"`
	CoverImageURL string `json:"coverImageUrl"`

	Category string `json:"category"`
	Timeline string `json:"timeline"`
	Role     string `json:"role"`

	Challenge string   `json:"challenge"`
	Solution  string   `json:"solution"`
	Results   []string `json:"results"`

	TechnicalDetails any `json:"technicalDetails"`

	DemoURL *string `json:"demoUrl,omitempty"`
	RepoURL *string `json:"repoUrl,omitempty"`

	IsFeatured bool `json:"isFeatured"`
	SortOrder  int  `json:"sortOrder"`

	Tags        []TagResponse               `json:"tags"`
	Features    []ProjectFeatureResponse    `json:"features"`
	Screenshots []ProjectScreenshotResponse `json:"screenshots"`
}

// Mapper dari models.Project ke ProjectResponse
func projectToResponse(p models.Project) ProjectResponse {
	tags := make([]TagResponse, 0, len(p.Tags))
	for _, t := range p.Tags {
		tags = append(tags, TagResponse{
			ID:   t.ID.String(),
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

	screenshots := make([]ProjectScreenshotResponse, 0, len(p.Screenshots))
	for _, s := range p.Screenshots {
		screenshots = append(screenshots, ProjectScreenshotResponse{
			ImageURL:  s.ImageURL,
			SortOrder: s.SortOrder,
		})
	}

	// karena di model pakai pq.StringArray, di sini sudah []string
	results := make([]string, len(p.Results))
	copy(results, p.Results)

	return ProjectResponse{
		ID:            p.ID.String(),
		Title:         p.Title,
		Slug:          p.Slug,
		ShortDesc:     p.ShortDesc,
		LongDesc:      p.LongDesc,
		CoverImageURL: p.CoverImageURL,

		Category: p.Category,
		Timeline: p.Timeline,
		Role:     p.Role,

		Challenge: p.Challenge,
		Solution:  p.Solution,
		Results:   results,

		TechnicalDetails: p.TechnicalDetails, // datatypes.JSON -> any

		DemoURL: p.DemoURL,
		RepoURL: p.RepoURL,

		IsFeatured: p.IsFeatured,
		SortOrder:  p.SortOrder,

		Tags:        tags,
		Features:    features,
		Screenshots: screenshots,
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
