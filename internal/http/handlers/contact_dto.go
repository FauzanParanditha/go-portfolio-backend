package handlers

import "github.com/FauzanParanditha/portfolio-backend/internal/models"

type ContactCreateRequest struct {
	Name    string `json:"name" validate:"required"`
	Email   string `json:"email" validate:"required,email"`
	Subject string `json:"subject" validate:"required"`
	Message string `json:"message" validate:"required"`
}

type ContactMessageResponse struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	Subject   string `json:"subject"`
	Message   string `json:"message"`
	IsRead    bool   `json:"isRead"`
	CreatedAt string `json:"createdAt"`
}

func contactToResponse(m models.ContactMessage) ContactMessageResponse {
	return ContactMessageResponse{
		ID:        m.ID.String(),
		Name:      m.Name,
		Email:     m.Email,
		Subject:   m.Subject,
		Message:   m.Message,
		IsRead:    m.IsRead,
		CreatedAt: m.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}
