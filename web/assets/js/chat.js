// Establecer conexión WebSocket
const ws = new WebSocket("ws://localhost:8080/ws");

const messageList = document.getElementById('message-list');
const messageForm = document.getElementById('message-form');
const usernameInput = document.getElementById('username');
const messageInput = document.getElementById('message');

// Evento que maneja los mensajes entrantes
ws.onmessage = function(event) {
    const msg = JSON.parse(event.data);
    displayMessage(msg.username, msg.content);
};

// Enviar mensaje cuando el formulario es enviado
messageForm.addEventListener('submit', function(event) {
    event.preventDefault();  // Evitar el comportamiento por defecto del formulario
    if (!usernameInput.value || !messageInput.value) return;  // Validar campos vacíos

    const message = {
        username: usernameInput.value,
        content: messageInput.value
    };

    // Enviar mensaje a través de WebSocket
    ws.send(JSON.stringify(message));

    // Mostrar el mensaje en el chat
    displayMessage(message.username, message.content);

    // Limpiar el campo de mensaje
    messageInput.value = '';
});

// Función para mostrar mensajes en la lista
function displayMessage(username, content) {
    const p = document.createElement('p');
    p.innerHTML = `<strong>${username}</strong>: ${content}`;
    messageList.appendChild(p);
    messageList.scrollTop = messageList.scrollHeight;  // Desplazar hacia abajo automáticamente
}
