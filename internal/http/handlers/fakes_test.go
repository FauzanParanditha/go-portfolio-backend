package handlers_test

import (
	"context"

	"github.com/FauzanParanditha/portfolio-backend/internal/models"
	"github.com/FauzanParanditha/portfolio-backend/internal/repository"
)

// File ini berisi fake repository (implementasi interface repository) yang
// dipakai bersama oleh test handler. Semua fake memakai field func agar tiap
// test bisa menentukan perilaku (data/eror) tanpa menyentuh database.

// --- fakeProjectRepo: implementasi repository.ProjectRepository ---

type fakeProjectRepo struct {
	listFn      func(ctx context.Context, params repository.ProjectListParams) ([]models.Project, int64, error)
	getBySlugFn func(ctx context.Context, slug string) (*models.Project, error)
}

func (f *fakeProjectRepo) ListPublic(ctx context.Context, params repository.ProjectListParams) ([]models.Project, int64, error) {
	return f.listFn(ctx, params)
}

func (f *fakeProjectRepo) GetBySlug(ctx context.Context, slug string) (*models.Project, error) {
	return f.getBySlugFn(ctx, slug)
}

// --- fakeExperienceRepo: implementasi repository.ExperienceRepository ---

type fakeExperienceRepo struct {
	listFn func(ctx context.Context) ([]models.Experience, error)
}

func (f *fakeExperienceRepo) ListPublic(ctx context.Context) ([]models.Experience, error) {
	return f.listFn(ctx)
}

// --- fakeContactRepo: implementasi repository.ContactMessageRepository ---

type fakeContactRepo struct {
	createFn   func(ctx context.Context, m *models.ContactMessage) error
	listFn     func(ctx context.Context, params repository.ContactListParams) ([]models.ContactMessage, int64, error)
	getByIDFn  func(ctx context.Context, id string) (*models.ContactMessage, error)
	markReadFn func(ctx context.Context, id string, isRead bool) error
	deleteFn   func(ctx context.Context, id string) error
}

func (f *fakeContactRepo) Create(ctx context.Context, m *models.ContactMessage) error {
	return f.createFn(ctx, m)
}

func (f *fakeContactRepo) List(ctx context.Context, params repository.ContactListParams) ([]models.ContactMessage, int64, error) {
	return f.listFn(ctx, params)
}

func (f *fakeContactRepo) GetByID(ctx context.Context, id string) (*models.ContactMessage, error) {
	return f.getByIDFn(ctx, id)
}

func (f *fakeContactRepo) MarkRead(ctx context.Context, id string, isRead bool) error {
	return f.markReadFn(ctx, id, isRead)
}

func (f *fakeContactRepo) Delete(ctx context.Context, id string) error {
	return f.deleteFn(ctx, id)
}

// --- fakeTagRepo: implementasi repository.TagRepository ---

type fakeTagRepo struct {
	listFn    func(ctx context.Context, params repository.TagListParams) ([]models.Tag, int64, error)
	getByIDFn func(ctx context.Context, id string) (*models.Tag, error)
	createFn  func(ctx context.Context, tag *models.Tag) error
	updateFn  func(ctx context.Context, tag *models.Tag) error
	deleteFn  func(ctx context.Context, id string) error
}

func (f *fakeTagRepo) List(ctx context.Context, params repository.TagListParams) ([]models.Tag, int64, error) {
	return f.listFn(ctx, params)
}

func (f *fakeTagRepo) GetByID(ctx context.Context, id string) (*models.Tag, error) {
	return f.getByIDFn(ctx, id)
}

func (f *fakeTagRepo) Create(ctx context.Context, tag *models.Tag) error {
	return f.createFn(ctx, tag)
}

func (f *fakeTagRepo) Update(ctx context.Context, tag *models.Tag) error {
	return f.updateFn(ctx, tag)
}

func (f *fakeTagRepo) Delete(ctx context.Context, id string) error {
	return f.deleteFn(ctx, id)
}

// Pastikan fake memenuhi kontrak interface pada waktu kompilasi.
var (
	_ repository.ProjectRepository        = (*fakeProjectRepo)(nil)
	_ repository.ExperienceRepository     = (*fakeExperienceRepo)(nil)
	_ repository.ContactMessageRepository = (*fakeContactRepo)(nil)
	_ repository.TagRepository            = (*fakeTagRepo)(nil)
)
