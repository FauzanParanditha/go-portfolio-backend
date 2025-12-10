APP_NAME=portfolio-backend
CMD_PATH=./cmd/api
ATLAS_ENV=dev

.PHONY: seed run build tidy migrate-new migrate-up migrate-down migrate-down-to migrate-down-all migrate-status fmt vet test

## ------- Development Commands ------- ##

## Jalankan aplikasi
run:
	go run $(CMD_PATH)

## Jalankan dengan live reload (butuh 'air')
dev:
	air

## Seed database
seed:
	go run ./cmd/seed

## Build binary
build:
	go build -o bin/$(APP_NAME) $(CMD_PATH)

## Go fmt + go vet
fmt:
	go fmt ./...
vet:
	go vet ./...

## Tidy dependency
tidy:
	go mod tidy


## ------- Database Migration (Atlas) ------- ##

## Buat migration baru
## contoh: make migrate-new name=add_project_subtitle
migrate-new:
ifndef name
	$(error name is required. Usage: make migrate-new name=add_project_subtitle)
endif
	atlas migrate new $(name) --env $(ATLAS_ENV)

## Apply migration
migrate-up:
	atlas migrate hash --env $(ATLAS_ENV)
	atlas migrate apply --env $(ATLAS_ENV)

## Rollback 1 step
migrate-down:
	atlas migrate down --env $(ATLAS_ENV)

## Rollback ke versi tertentu
## contoh: make migrate-down-to version=20250115123001
migrate-down-to:
ifndef version
	$(error version is required. Usage: make migrate-down-to version=xxxx)
endif
	atlas migrate down --to $(version) --env $(ATLAS_ENV)

## Rollback semua migrasi
migrate-down-all:
	atlas migrate down --all --env $(ATLAS_ENV)

## Cek status migration
migrate-status:
	atlas migrate status --env $(ATLAS_ENV)


## ------- Swagger ------- ##
## swagger init
swagger:
	swag init -g cmd/api/main.go -o docs