package handlers

import (
	"github.com/FauzanParanditha/portfolio-backend/internal/models"
)

type ExperienceHighlightResponse struct {
	Text string `json:"text"`
}

type ExperienceResponse struct {
	ID          string                       `json:"id"`
	Title       string                       `json:"title"`
	Company     string                       `json:"company"`
	Location    string                       `json:"location,omitempty"`
	StartDate   string                       `json:"startDate"`           // "2006-01-02"
	EndDate     *string                      `json:"endDate,omitempty"`   // nullable
	IsCurrent   bool                         `json:"isCurrent"`
	Description string                       `json:"description,omitempty"`
	SortOrder   int                          `json:"sortOrder"`
	Tags        []TagResponse                `json:"tags"`
	Highlights  []ExperienceHighlightResponse `json:"highlights"`
}

type ExperienceCreateRequest struct {
	Title       string   `json:"title" validate:"required"`
	Company     string   `json:"company" validate:"required"`
	Location    string   `json:"location"`
	StartDate   string   `json:"startDate" validate:"required"` // "2006-01-02"
	EndDate     *string  `json:"endDate"`                       // nullable
	IsCurrent   bool     `json:"isCurrent"`
	Description string   `json:"description"`
	SortOrder   int      `json:"sortOrder"`
	TagIDs      []string `json:"tagIds"`      // UUID string
	Highlights  []string `json:"highlights"`  // teks bullet
}

type ExperienceUpdateRequest = ExperienceCreateRequest

func experienceToResponse(e models.Experience) ExperienceResponse {
	tags := make([]TagResponse, 0, len(e.Tags))
	for _, t := range e.Tags {
		tags = append(tags, TagResponse{
			Name: t.Name,
			Type: t.Type,
		})
	}

	highs := make([]ExperienceHighlightResponse, 0, len(e.Highlights))
	for _, h := range e.Highlights {
		highs = append(highs, ExperienceHighlightResponse{
			Text: h.Text,
		})
	}

	start := e.StartDate.Format("2006-01-02")

	var endStr *string
	if e.EndDate != nil {
		s := e.EndDate.Format("2006-01-02")
		endStr = &s
	}

	return ExperienceResponse{
		ID:          e.ID.String(),
		Title:       e.Title,
		Company:     e.Company,
		Location:    e.Location,
		StartDate:   start,
		EndDate:     endStr,
		IsCurrent:   e.IsCurrent,
		Description: e.Description,
		SortOrder:   e.SortOrder,
		Tags:        tags,
		Highlights:  highs,
	}
}

// helper parse date "2006-01-02"

