# GoChat Makefile
# Ejecutar desde la raiz del proyecto (go-chat/)

.PHONY: help start stop dev build test clean

# Mostrar comandos disponibles
help:
	@echo "GoChat - Comandos disponibles:"
	@echo "  start  - Levantar Redis y ejecutar aplicacion"
	@echo "  dev    - Modo desarrollo con Redis"
	@echo "  stop   - Parar Redis"
	@echo "  build  - Compilar aplicacion"
	@echo "  test   - Ejecutar tests"
	@echo "  clean  - Limpiar contenedores y binarios"

# Verificar que estamos en el directorio correcto
check-dir:
	@test -d chat-app || { echo "Error: Directorio chat-app no encontrado. Ejecutar desde raiz del proyecto."; exit 1; }
	@test -f chat-app/go.mod || { echo "Error: go.mod no encontrado en chat-app/."; exit 1; }

# Levantar Redis y ejecutar aplicacion
start: check-dir
	docker-compose up -d redis
	sleep 3
	cd chat-app && go run cmd/main.go

# Modo desarrollo
dev: check-dir
	docker-compose up -d redis
	sleep 3
	cd chat-app && go run cmd/main.go

# Parar servicios
stop:
	docker-compose down

# Compilar aplicacion
build: check-dir
	mkdir -p bin
	cd chat-app && go build -o ../bin/gochat cmd/main.go

# Ejecutar tests
test: check-dir
	cd chat-app && go test ./...

# Descargar dependencias
deps: check-dir
	cd chat-app && go mod tidy && go mod download

# Limpiar todo
clean:
	docker-compose down -v
	rm -rf bin/

# Ver logs de Redis
logs:
	docker-compose logs -f redis