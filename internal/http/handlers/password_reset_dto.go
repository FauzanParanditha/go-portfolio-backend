package handlers

// ForgotPasswordRequest adalah body untuk POST /auth/forgot-password.
type ForgotPasswordRequest struct {
	Email string `json:"email" validate:"required,email"`
}

// ResetPasswordRequest adalah body untuk POST /auth/reset-password.
// Token berasal dari tautan email; password adalah password baru.
type ResetPasswordRequest struct {
	Token    string `json:"token" validate:"required"`
	Password string `json:"password" validate:"required,min=8,max=72"`
}

// MessageResponse adalah envelope sukses sederhana untuk endpoint yang tidak
// mengembalikan resource, hanya konfirmasi.
type MessageResponse struct {
	Message string `json:"message"`
}
