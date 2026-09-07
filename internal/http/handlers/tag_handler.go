package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/FauzanParanditha/portfolio-backend/internal/repository"
	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog/log"
)

// publicTagLimit adalah batas atas jumlah tag yang dikembalikan ke publik.
//
// Endpoint ini dipakai halaman "About" untuk menampilkan daftar keahlian, jadi
// tidak perlu paginasi — tapi tetap dibatasi supaya jumlah tag yang membengkak
// tidak diam-diam berubah menjadi response raksasa.
const publicTagLimit = 200

type TagHandler struct {
	repo repository.TagRepository
}

func NewTagHandler(repo repository.TagRepository) *TagHandler {
	return &TagHandler{repo: repo}
}

// GET /api/v1/tags
// Public List Tags godoc
// @Summary      Daftar tag publik
// @Description  Seluruh tag beserta tipenya, dipakai situs publik untuk menyusun daftar keahlian. Read-only; pembuatan dan perubahan tetap lewat endpoint admin.
// @Tags         tags
// @Produce      json
// @Success      200  {array}  TagResponse
// @Failure      500  {object} ErrorResponse
// @Router       /tags [get]
func (h *TagHandler) List(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tags, _, err := h.repo.List(ctx, repository.TagListParams{
		Page:  1,
		Limit: publicTagLimit,
	})
	if err != nil {
		log.Error().Err(err).Msg("failed to list tags (public)")
		return fiber.NewError(http.StatusInternalServerError, "failed to fetch tags")
	}

	resp := make([]TagResponse, 0, len(tags))
	for _, t := range tags {
		resp = append(resp, tagToResponse(t))
	}

	return c.JSON(fiber.Map{"data": resp})
}
