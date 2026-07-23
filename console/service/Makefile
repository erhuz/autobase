ifndef GO_BIN
override GO_BIN = "pg-console"
endif

APP = main.go

swagger_install:
	@go install github.com/go-swagger/go-swagger/cmd/swagger@v0.35.3

ensure_deps: ## Ensure Go module dependencies are tidy
	@go mod tidy

build: ## Build app
	@go build -o $(GO_BIN) $(APP)

test: ## Test with the committed module lock
	@go test -mod=readonly ./migrations ./internal/configuration ./internal/storage

swagger:
	@swagger generate server --name PgConsole --spec api/swagger.yaml --principal interface{} --exclude-main

build_in_docker: swagger_install swagger ensure_deps build
