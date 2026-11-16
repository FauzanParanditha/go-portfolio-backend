package handlers

import "github.com/FauzanParanditha/portfolio-backend/internal/models"

// ContactCreateRequest adalah body form kontak publik.
//
// Batas panjang (dihitung per karakter, bukan byte): endpoint ini publik tanpa
// auth, jadi tanpa batas siapa pun bisa mengirim pesan sebesar body limit (1 MB)
// per request. Batas juga harus <= kolom DB (migrations: name varchar(150),
// email varchar(200), subject varchar(200), message text); bila melebihi kolom,
// input lolos validasi lalu gagal di Postgres sebagai 500, bukan 422 yang
// jelas. Karena itu email dibatasi 200 (kolom DB), bukan 254 (batas RFC).
type ContactCreateRequest struct {
	Name    string `json:"name" validate:"required,max=100"`
	Email   string `json:"email" validate:"required,max=200,email"`
	Subject string `json:"subject" validate:"required,max=200"`
	Message string `json:"message" validate:"required,max=5000"`
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
