# 🚀 GoChat - Real-time Chat Application

> **Aplicación de chat en tiempo real construida con Go, WebSockets y Redis**

Una solución completa de chat que demuestra arquitectura backend escalable, comunicación en tiempo real y gestión eficiente de estado distribuido.

## ✨ Características Principales

- **💬 Chat en tiempo real** - Mensajes instantáneos entre usuarios
- **🏠 Salas dinámicas** - Creación y cambio de salas sobre la marcha
- **👥 Lista de usuarios activos** - Visualización en tiempo real de quién está conectado
- **⌨️ Indicador de escritura** - Muestra cuando alguien está escribiendo
- **📊 Estadísticas en vivo** - Conteo de usuarios y mensajes por sala
- **💾 Historial persistente** - Mensajes recientes almacenados en Redis
- **🔄 Reconexión automática** - Manejo robusto de desconexiones

## 🛠️ Stack Tecnológico

| Componente | Tecnología | Propósito |
|------------|------------|-----------|
| **Backend** | Go 1.24 | Server HTTP/WebSocket |
| **WebSockets** | Gorilla WebSocket | Comunicación tiempo real |
| **Cache/Store** | Redis | Persistencia y estado distribuido |
| **Frontend** | HTML5/CSS3/JavaScript | Interfaz de usuario reactiva |
| **Containerización** | Docker Compose | Orquestación de servicios |

## 🏗️ Arquitectura del Sistema

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   Web Client    │◄──►│   Go Server     │◄──►│     Redis       │
│                 │    │                 │    │                 │
│ • HTML/CSS/JS   │    │ • HTTP Handler  │    │ • Message Store │
│ • WebSocket     │    │ • WebSocket Hub │    │ • User Sessions │
│ • Auto-reconnect│    │ • Room Manager  │    │ • Room Stats    │
└─────────────────┘    └─────────────────┘    └─────────────────┘
```

### Flujo de Comunicación

1. **Cliente** se conecta vía WebSocket al servidor Go
2. **Hub central** gestiona todas las conexiones y salas
3. **Redis** persiste mensajes y mantiene estado de usuarios
4. **Broadcast** distribuye mensajes a usuarios de la misma sala
5. **Estado sincronizado** entre múltiples instancias del servidor

## 🚀 Quick Start

### Prerrequisitos
- Docker & Docker Compose
- Go 1.24+ (para desarrollo)

### Ejecutar en 30 segundos

```bash
# Clonar el repositorio
git clone https://github.com/tu-usuario/go-chat.git
cd go-chat

# Levantar la aplicación completa
make start

# Abrir en navegador
open http://localhost:8080
```

### Comandos Disponibles

```bash
make start    # Levantar Redis + Aplicación
make dev      # Modo desarrollo con hot reload
make stop     # Parar todos los servicios
make build    # Compilar ejecutable
make test     # Ejecutar tests
make clean    # Limpiar contenedores y builds
```

## 📱 Uso de la Aplicación

### 1. **Conexión Inicial**
- Introduce tu **username**
- Especifica una **sala** (ej: `general`, `tech`, `random`)
- Haz clic en **"Join Chat"**

### 2. **Funcionalidades del Chat**
- **Enviar mensajes** - Escribe y presiona Enter
- **Cambiar de sala** - Usa el panel lateral o comando `/join <sala>`
- **Ver estadísticas** - Comando `/stats [sala]`
- **Indicador de escritura** - Automático al escribir

### 3. **Características Avanzadas**
- **Historial** - Los últimos 20 mensajes se cargan automáticamente
- **Reconexión** - La app se reconecta automáticamente si se pierde conexión
- **Multi-ventana** - Abre múltiples pestañas para simular usuarios

## 🏛️ Arquitectura de Código

### Estructura del Proyecto
```
go-chat/
├── cmd/main.go              # Punto de entrada
├── internal/
│   ├── websocket/           # Core WebSocket logic
│   │   ├── hub.go          # Gestor central de conexiones
│   │   ├── client.go       # Manejo individual de clientes  
│   │   └── message.go      # Tipos de mensajes
│   ├── handlers/           # HTTP request handlers
│   │   └── chat.go        # WebSocket upgrade & API
│   └── cache/             # Redis operations
│       └── redis.go       # Persistencia y cache
└── web/                   # Frontend assets
    ├── index.html        # UI principal
    ├── style.css         # Estilos modernos
    └── chat.js          # Lógica WebSocket cliente
```

### Patrones Implementados

- **🎯 Hexagonal Architecture** - Separación clara de capas
- **📡 Publisher/Subscriber** - Hub central para broadcast
- **🔄 Channel-based Communication** - Goroutines coordinadas via channels
- **🏪 Repository Pattern** - Abstracción de persistencia Redis
- **🛡️ Graceful Error Handling** - Recuperación de errores y logging

## 🔧 Configuración Avanzada

### Variables de Entorno

```bash
REDIS_URL=redis://localhost:6379/0  # URL de conexión Redis
PORT=8080                           # Puerto del servidor HTTP
```

### Desarrollo Local

```bash
# Levantar solo Redis (para desarrollo del backend)
docker-compose up -d redis

# Ejecutar aplicación con hot reload
cd chat-app && go run cmd/main.go

# Ejecutar tests
cd chat-app && go test ./...
```

## 📈 Métricas y Monitoring

La aplicación expone métricas en tiempo real:

- **`/api/rooms/stats?roomID=<sala>`** - Estadísticas por sala
- **Logs estructurados** - Formato consistente para monitoring
- **Health checks** - Redis connection monitoring

## 🤝 Contribución

1. Fork del proyecto
2. Crear feature branch (`git checkout -b feature/nueva-funcionalidad`)
3. Commit cambios (`git commit -am 'Añadir nueva funcionalidad'`)
4. Push al branch (`git push origin feature/nueva-funcionalidad`)
5. Crear Pull Request

## 📄 Licencia

MIT License - ver [LICENSE](LICENSE) para más detalles.

---

### 🎯 **Casos de Uso Demostrados**

- **Sistemas distribuidos** con estado compartido
- **Comunicación en tiempo real** escalable
- **Arquitectura de microservicios** preparada para Kubernetes
- **Testing de aplicaciones concurrentes** con Go
- **Gestión de estado frontend** sin frameworks pesados
