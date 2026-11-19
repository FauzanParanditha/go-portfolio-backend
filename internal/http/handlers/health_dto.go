package handlers

// HealthStatus adalah isi `data` pada respons endpoint status publik.
// Sengaja hanya satu field: status database tidak dirinci ke publik.
type HealthStatus struct {
	Status string `json:"status" enums:"ok,ready" example:"ok"`
}

// HealthResponse adalah envelope sukses endpoint status publik.
type HealthResponse struct {
	Data HealthStatus `json:"data"`
}
