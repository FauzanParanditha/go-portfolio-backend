package handlers

import (
	"errors"
	"io"
	"net/http"

	"github.com/FauzanParanditha/portfolio-backend/internal/storage"
	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog/log"
)

// UploadResponse adalah bentuk balasan setelah berkas tersimpan.
type UploadResponse struct {
	// URL siap ditempel ke field seperti `coverImageUrl`.
	URL         string `json:"url"`
	Filename    string `json:"filename"`
	ContentType string `json:"contentType"`
	Size        int64  `json:"size"`
}

type AdminUploadHandler struct {
	store storage.Storage
}

func NewAdminUploadHandler(store storage.Storage) *AdminUploadHandler {
	return &AdminUploadHandler{store: store}
}

// POST /api/v1/admin/uploads
// Admin Upload File godoc
// @Summary      Unggah berkas (gambar proyek / CV)
// @Description  Menerima satu berkas lewat multipart form field `file`, lalu mengembalikan URL publiknya untuk ditempel ke field seperti `coverImageUrl`. Jenis berkas ditentukan dari ISI berkas, bukan dari header Content-Type kiriman klien. Yang diizinkan: JPEG, PNG, WebP, GIF, dan PDF — SVG sengaja ditolak karena bisa memuat skrip.
// @Tags         admin-uploads
// @Security     BearerAuth
// @Accept       multipart/form-data
// @Produce      json
// @Param        file formData file true "Berkas yang diunggah"
// @Success      201  {object} UploadResponse
// @Failure      400  {object} ErrorResponse
// @Failure      401  {object} ErrorResponse
// @Failure      403  {object} ErrorResponse
// @Failure      413  {object} ErrorResponse
// @Failure      415  {object} ErrorResponse
// @Router       /admin/uploads [post]
func (h *AdminUploadHandler) Upload(c *fiber.Ctx) error {
	fh, err := c.FormFile("file")
	if err != nil {
		return fiber.NewError(http.StatusBadRequest, "berkas tidak ditemukan pada field 'file'")
	}

	f, err := fh.Open()
	if err != nil {
		log.Error().Err(err).Msg("gagal membuka berkas unggahan")
		return fiber.NewError(http.StatusInternalServerError, "gagal membaca berkas")
	}
	defer func() { _ = f.Close() }()

	data, err := io.ReadAll(f)
	if err != nil {
		log.Error().Err(err).Msg("gagal membaca isi berkas unggahan")
		return fiber.NewError(http.StatusInternalServerError, "gagal membaca berkas")
	}

	saved, err := h.store.Save(data, fh.Filename)
	if err != nil {
		switch {
		case errors.Is(err, storage.ErrTooLarge):
			return fiber.NewError(http.StatusRequestEntityTooLarge, "ukuran berkas melebihi batas")
		case errors.Is(err, storage.ErrTypeNotAllowed):
			// Pesannya menyebut jenis yang terdeteksi supaya jelas kenapa ditolak.
			return fiber.NewError(http.StatusUnsupportedMediaType, err.Error())
		default:
			log.Error().Err(err).Msg("gagal menyimpan berkas unggahan")
			return fiber.NewError(http.StatusInternalServerError, "gagal menyimpan berkas")
		}
	}

	log.Info().
		Str("filename", saved.Filename).
		Str("content_type", saved.ContentType).
		Int64("size", saved.Size).
		Msg("berkas unggahan tersimpan")

	return c.Status(http.StatusCreated).JSON(fiber.Map{
		"data": UploadResponse{
			URL:         saved.URL,
			Filename:    saved.Filename,
			ContentType: saved.ContentType,
			Size:        saved.Size,
		},
	})
}
