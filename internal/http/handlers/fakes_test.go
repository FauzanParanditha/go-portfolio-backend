package handlers_test

import (
	"context"
	"time"

	"github.com/FauzanParanditha/portfolio-backend/internal/mailer"
	"github.com/FauzanParanditha/portfolio-backend/internal/models"
	"github.com/FauzanParanditha/portfolio-backend/internal/repository"
	"github.com/google/uuid"
)

// File ini berisi fake repository (implementasi interface repository) yang
// dipakai bersama oleh test handler. Semua fake memakai field func agar tiap
// test bisa menentukan perilaku (data/eror) tanpa menyentuh database.

// --- fakeProjectRepo: implementasi repository.ProjectRepository ---

type fakeProjectRepo struct {
	listFn         func(ctx context.Context, params repository.ProjectListParams) ([]models.Project, int64, error)
	getBySlugFn    func(ctx context.Context, slug string) (*models.Project, error)
	listAdminFn    func(ctx context.Context, params repository.ProjectAdminListParams) ([]models.Project, int64, error)
	getByIDAdminFn func(ctx context.Context, id uuid.UUID) (*models.Project, error)
	createFn       func(ctx context.Context, in repository.ProjectWriteInput) (*models.Project, error)
	updateFn       func(ctx context.Context, id uuid.UUID, in repository.ProjectWriteInput) (*models.Project, error)
	deleteFn       func(ctx context.Context, id uuid.UUID) error
}

func (f *fakeProjectRepo) ListPublic(ctx context.Context, params repository.ProjectListParams) ([]models.Project, int64, error) {
	return f.listFn(ctx, params)
}

func (f *fakeProjectRepo) GetBySlug(ctx context.Context, slug string) (*models.Project, error) {
	return f.getBySlugFn(ctx, slug)
}

func (f *fakeProjectRepo) ListAdmin(ctx context.Context, params repository.ProjectAdminListParams) ([]models.Project, int64, error) {
	return f.listAdminFn(ctx, params)
}

func (f *fakeProjectRepo) GetByIDAdmin(ctx context.Context, id uuid.UUID) (*models.Project, error) {
	return f.getByIDAdminFn(ctx, id)
}

func (f *fakeProjectRepo) Create(ctx context.Context, in repository.ProjectWriteInput) (*models.Project, error) {
	return f.createFn(ctx, in)
}

func (f *fakeProjectRepo) Update(ctx context.Context, id uuid.UUID, in repository.ProjectWriteInput) (*models.Project, error) {
	return f.updateFn(ctx, id, in)
}

func (f *fakeProjectRepo) Delete(ctx context.Context, id uuid.UUID) error {
	return f.deleteFn(ctx, id)
}

// --- fakeExperienceRepo: implementasi repository.ExperienceRepository ---

type fakeExperienceRepo struct {
	listFn         func(ctx context.Context) ([]models.Experience, error)
	listAdminFn    func(ctx context.Context, params repository.ExperienceAdminListParams) ([]models.Experience, int64, error)
	getByIDAdminFn func(ctx context.Context, id uuid.UUID) (*models.Experience, error)
	createFn       func(ctx context.Context, in repository.ExperienceWriteInput) (*models.Experience, error)
	updateFn       func(ctx context.Context, id uuid.UUID, in repository.ExperienceWriteInput) (*models.Experience, error)
	deleteFn       func(ctx context.Context, id uuid.UUID) error
}

func (f *fakeExperienceRepo) ListPublic(ctx context.Context) ([]models.Experience, error) {
	return f.listFn(ctx)
}

func (f *fakeExperienceRepo) ListAdmin(ctx context.Context, params repository.ExperienceAdminListParams) ([]models.Experience, int64, error) {
	return f.listAdminFn(ctx, params)
}

func (f *fakeExperienceRepo) GetByIDAdmin(ctx context.Context, id uuid.UUID) (*models.Experience, error) {
	return f.getByIDAdminFn(ctx, id)
}

func (f *fakeExperienceRepo) Create(ctx context.Context, in repository.ExperienceWriteInput) (*models.Experience, error) {
	return f.createFn(ctx, in)
}

func (f *fakeExperienceRepo) Update(ctx context.Context, id uuid.UUID, in repository.ExperienceWriteInput) (*models.Experience, error) {
	return f.updateFn(ctx, id, in)
}

func (f *fakeExperienceRepo) Delete(ctx context.Context, id uuid.UUID) error {
	return f.deleteFn(ctx, id)
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

// --- fakeUserRepo: implementasi repository.UserRepository ---

type fakeUserRepo struct {
	findByEmailFn    func(ctx context.Context, email string) (*models.User, error)
	findByIDFn       func(ctx context.Context, id string) (*models.User, error)
	updatePasswordFn func(ctx context.Context, id uuid.UUID, hashedPassword string) error
}

func (f *fakeUserRepo) FindByEmail(ctx context.Context, email string) (*models.User, error) {
	return f.findByEmailFn(ctx, email)
}

func (f *fakeUserRepo) FindByID(ctx context.Context, id string) (*models.User, error) {
	return f.findByIDFn(ctx, id)
}

func (f *fakeUserRepo) UpdatePassword(ctx context.Context, id uuid.UUID, hashedPassword string) error {
	if f.updatePasswordFn == nil {
		return nil
	}
	return f.updatePasswordFn(ctx, id, hashedPassword)
}

// --- fakePasswordResetRepo: implementasi repository.PasswordResetRepository ---

type fakePasswordResetRepo struct {
	createFn               func(ctx context.Context, t *models.PasswordResetToken) error
	findValidByHashFn      func(ctx context.Context, tokenHash string) (*models.PasswordResetToken, error)
	markUsedFn             func(ctx context.Context, id uuid.UUID) error
	invalidateAllForUserFn func(ctx context.Context, userID uuid.UUID) error
	deleteExpiredFn        func(ctx context.Context) error
}

func (f *fakePasswordResetRepo) Create(ctx context.Context, t *models.PasswordResetToken) error {
	if f.createFn == nil {
		return nil
	}
	return f.createFn(ctx, t)
}

func (f *fakePasswordResetRepo) FindValidByHash(ctx context.Context, tokenHash string) (*models.PasswordResetToken, error) {
	return f.findValidByHashFn(ctx, tokenHash)
}

func (f *fakePasswordResetRepo) MarkUsed(ctx context.Context, id uuid.UUID) error {
	if f.markUsedFn == nil {
		return nil
	}
	return f.markUsedFn(ctx, id)
}

func (f *fakePasswordResetRepo) InvalidateAllForUser(ctx context.Context, userID uuid.UUID) error {
	if f.invalidateAllForUserFn == nil {
		return nil
	}
	return f.invalidateAllForUserFn(ctx, userID)
}

func (f *fakePasswordResetRepo) DeleteExpired(ctx context.Context) error {
	if f.deleteExpiredFn == nil {
		return nil
	}
	return f.deleteExpiredFn(ctx)
}

// --- fakeMailer: implementasi mailer.Mailer, merekam email terakhir ---

type fakeMailer struct {
	enabled bool
	sendErr error

	sentTo      string
	sentSubject string
	sentBody    string
	sendCount   int
}

func (f *fakeMailer) Enabled() bool { return f.enabled }

func (f *fakeMailer) Send(_ context.Context, to, subject, body string) error {
	f.sendCount++
	f.sentTo, f.sentSubject, f.sentBody = to, subject, body
	return f.sendErr
}

// --- fakeDashboardRepo: implementasi repository.DashboardRepository ---

type fakeDashboardRepo struct {
	overviewFn func(ctx context.Context, since time.Time) (repository.DashboardCounts, error)
}

func (f *fakeDashboardRepo) Overview(ctx context.Context, since time.Time) (repository.DashboardCounts, error) {
	return f.overviewFn(ctx, since)
}

// Pastikan fake memenuhi kontrak interface pada waktu kompilasi.
var (
	_ repository.ProjectRepository        = (*fakeProjectRepo)(nil)
	_ repository.ExperienceRepository     = (*fakeExperienceRepo)(nil)
	_ repository.ContactMessageRepository = (*fakeContactRepo)(nil)
	_ repository.TagRepository            = (*fakeTagRepo)(nil)
	_ repository.UserRepository           = (*fakeUserRepo)(nil)
	_ repository.DashboardRepository      = (*fakeDashboardRepo)(nil)
	_ repository.PasswordResetRepository  = (*fakePasswordResetRepo)(nil)
	_ mailer.Mailer                       = (*fakeMailer)(nil)
)
