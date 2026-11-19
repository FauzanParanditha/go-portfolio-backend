package handlers

// Isi `data` pada respons endpoint status publik. Sengaja hanya satu field:
// status database tidak dirinci ke publik.
//
// Dua tipe terpisah (bukan satu tipe bersama) supaya contoh dan enum di Swagger
// tepat per endpoint: `/healthz` selalu "ok", `/readyz` selalu "ready".

// HealthzStatus adalah isi `data` pada GET /api/v1/healthz.
type HealthzStatus struct {
	Status string `json:"status" enums:"ok" example:"ok"`
}

// HealthzResponse adalah envelope sukses GET /api/v1/healthz.
type HealthzResponse struct {
	Data HealthzStatus `json:"data"`
}

// ReadyzStatus adalah isi `data` pada GET /api/v1/readyz.
type ReadyzStatus struct {
	Status string `json:"status" enums:"ready" example:"ready"`
}

// ReadyzResponse adalah envelope sukses GET /api/v1/readyz.
type ReadyzResponse struct {
	Data ReadyzStatus `json:"data"`
}
