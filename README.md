# GoChat

GoChat is a real-time chat application built in Go using WebSockets. It allows users to connect and send messages in a web environment.

## Project Structure
```
/GoChat
 ├── /cmd
 │   └── main.go           # Punto de entrada de la aplicación
 ├── /internal
 │   └── /websocket
 │       └── websocket.go   # Lógica para manejar las conexiones WebSocket
 ├── /web
 │   ├── /static
 │   │   ├── index.html     # Archivo HTML principal
 │   │   └── styles.css      # Estilos CSS
 │   └── /assets
 │       └── /js
 │           └── chat.js     # Lógica del chat en JavaScript
```
## Features

- **Real-time chat**: Messages are sent and received instantly.
- **Simple interface**: A minimalist design that makes it easy for users to interact.

## Technologies Used

- **Go**: The main programming language for the backend.
- **WebSockets**: For real-time communication between the server and clients.
- **HTML/CSS/JavaScript**: For building the user interface.

## How to Run

1. Clone the repository.
2. Navigate to the project folder.
3. Run the server with the following command:

   ```bash
   go run cmd/main.go
   
Abre tu navegador y ve a http://localhost:8080