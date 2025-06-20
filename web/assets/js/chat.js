document.addEventListener('DOMContentLoaded', () => {
    // View elements
    const loginView = document.getElementById('login-view');
    const chatView = document.getElementById('chat-view');

    // Login elements
    const loginForm = document.getElementById('login-form');
    const usernameInput = document.getElementById('username');
    const passwordInput = document.getElementById('password');
    const loginError = document.getElementById('login-error');

    // Chat elements
    const currentUserIdDisplay = document.getElementById('current-user-id-display');
    const logoutBtn = document.getElementById('logout-btn');

    const roomIdInput = document.getElementById('room-id-input');
    const joinRoomBtn = document.getElementById('join-room-btn');
    const currentRoomDisplay = document.getElementById('current-room-display');

    const messagesArea = document.getElementById('messages-area');
    const messageForm = document.getElementById('message-form');
    const messageInput = document.getElementById('message-input');

    // State
    let accessToken = localStorage.getItem('accessToken');
    let refreshToken = localStorage.getItem('refreshToken');
    let userID = localStorage.getItem('userID');
    let currentRoomID = null;
    let ws = null;

    // Initial UI setup
    updateUI();

    // Event Listeners
    loginForm.addEventListener('submit', handleLogin);
    logoutBtn.addEventListener('click', handleLogout);
    joinRoomBtn.addEventListener('click', handleJoinRoom);
    messageForm.addEventListener('submit', handleSendMessage);

    function updateUI() {
        if (accessToken) {
            loginView.style.display = 'none';
            chatView.style.display = 'block';
            currentUserIdDisplay.textContent = userID || 'N/A';
            currentRoomDisplay.textContent = currentRoomID || 'N/A';
            if (!ws || ws.readyState === WebSocket.CLOSED) {
                connectWebSocket();
            }
        } else {
            loginView.style.display = 'block';
            chatView.style.display = 'none';
            currentUserIdDisplay.textContent = 'N/A';
            currentRoomDisplay.textContent = 'N/A';
        }
    }

    async function handleLogin(event) {
        event.preventDefault();
        const username = usernameInput.value;
        const password = passwordInput.value;
        loginError.textContent = '';

        try {
            const response = await fetch('/api/v1/users/login', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ username, password }),
            });

            const data = await response.json();
            if (!response.ok) {
                throw new Error(data.error || `HTTP error! status: ${response.status}`);
            }

            accessToken = data.access_token;
            refreshToken = data.refresh_token;
            userID = data.user ? data.user.id : 'Unknown'; // Assuming user object with id is returned

            localStorage.setItem('accessToken', accessToken);
            localStorage.setItem('refreshToken', refreshToken);
            localStorage.setItem('userID', userID);

            updateUI();

        } catch (error) {
            console.error('Login failed:', error);
            loginError.textContent = error.message;
        }
    }

    function handleLogout() {
        accessToken = null;
        refreshToken = null;
        userID = null;
        currentRoomID = null; // Also clear current room on logout

        localStorage.removeItem('accessToken');
        localStorage.removeItem('refreshToken');
        localStorage.removeItem('userID');

        if (ws) {
            ws.close();
            ws = null;
        }
        updateUI();
        messagesArea.innerHTML = ''; // Clear messages
        loginError.textContent = ''; // Clear any previous login errors
        usernameInput.value = ''; // Clear username field
        passwordInput.value = ''; // Clear password field
    }

    function connectWebSocket() {
        if (!accessToken) {
            console.error('No access token available for WebSocket connection.');
            // TODO: Maybe try to refresh token here if refresh token is available
            return;
        }
        if (ws && (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING)) {
            console.log('WebSocket already open or connecting.');
            return;
        }

        const wsUrl = `ws://${window.location.host}/ws?token=${accessToken}`;
        ws = new WebSocket(wsUrl);

        ws.onopen = () => {
            console.log('WebSocket connection established.');
            displaySystemMessage('Connected to server.');
            // If a room was previously joined or input, try to join it
            if (currentRoomID) {
                 sendJoinRoomMessage(currentRoomID);
            } else if (roomIdInput.value) { // Or if there's a value in input field
                handleJoinRoom(); // This will set currentRoomID and send join message
            }
        };

        ws.onmessage = (event) => {
            try {
                const message = JSON.parse(event.data);
                console.log('Received message:', message);

                switch (message.type) {
                    case 'new_message':
                        // Payload of new_message is expected to be a domain.Message like structure
                        const msgPayload = message.payload; // This is already an object if server sends it as such
                        displayChatMessage(msgPayload.UserID, msgPayload.content, msgPayload.timestamp, msgPayload.roomID);
                        break;
                    case 'user_joined':
                    case 'user_left':
                        // Payload for these is map[string]string{"user_id": "...", "content": "...", "room_id": "..."}
                        const systemMsgPayload = message.payload;
                        if (systemMsgPayload.room_id === currentRoomID) {
                             displaySystemMessage(systemMsgPayload.content);
                        }
                        break;
                    // Add more cases for other message types from the hub
                    default:
                        displaySystemMessage(`Unknown message type: ${message.type}`);
                }
            } catch (e) {
                console.error('Error parsing message or unknown message structure:', e, event.data);
                displaySystemMessage(`Received raw: ${event.data}`);
            }
        };

        ws.onerror = (error) => {
            console.error('WebSocket error:', error);
            displaySystemMessage('WebSocket error. Check console.');
            // TODO: More robust error handling, e.g. token refresh on 401 from ws upgrade
        };

        ws.onclose = (event) => {
            console.log('WebSocket connection closed:', event.code, event.reason);
            displaySystemMessage(`Disconnected. Code: ${event.code}. ${event.reason ? "Reason: " + event.reason : ""}`);
            ws = null; // Clear the ws variable
            // TODO: Implement reconnection logic, possibly with token refresh
            // For now, user has to log out and log in again or refresh page if token expired
            // If it was an unexpected close, maybe try to reconnect after a delay
            if (event.code === 1006 && accessToken) { // Abnormal closure, and we were logged in
                console.log("Attempting to reconnect WebSocket after 5 seconds...");
                setTimeout(connectWebSocket, 5000);
            } else if (event.code === 1000 && !accessToken) { // Normal close after logout
                // Do nothing
            } else {
                 // Potentially token expired, or other issue
                 // For now, just log out the user to force re-login
                 // handleLogout();
                 // updateUI();
            }
        };
    }

    function sendJoinRoomMessage(roomID) {
        if (!ws || ws.readyState !== WebSocket.OPEN) {
            displaySystemMessage('WebSocket not connected. Cannot join room.');
            return;
        }
        const message = {
            type: 'join_room',
            payload: { roomID: roomID }
        };
        ws.send(JSON.stringify(message));
        currentRoomID = roomID; // Assume join is successful, hub might send confirmation/rejection
        currentRoomDisplay.textContent = currentRoomID;
        displaySystemMessage(`Attempting to join room: ${roomID}`);
        messagesArea.innerHTML = ''; // Clear messages from previous room
    }

    function handleJoinRoom() {
        const roomIDToJoin = roomIdInput.value.trim();
        if (!roomIDToJoin) {
            alert('Please enter a Room ID.');
            return;
        }
        if (currentRoomID === roomIDToJoin && ws && ws.readyState === WebSocket.OPEN) {
            displaySystemMessage(`Already in room: ${roomIDToJoin}`);
            return;
        }
        sendJoinRoomMessage(roomIDToJoin);
    }

    function handleSendMessage(event) {
        event.preventDefault();
        const messageText = messageInput.value.trim();

        if (!messageText) return;
        if (!currentRoomID) {
            displaySystemMessage('Please join a room first to send messages.');
            return;
        }
        if (!ws || ws.readyState !== WebSocket.OPEN) {
            displaySystemMessage('WebSocket not connected. Cannot send message.');
            return;
        }

        const message = {
            type: 'send_message',
            payload: {
                roomID: currentRoomID,
                content: messageText
            }
            // ClientID (UserID) will be set by the hub based on authenticated WebSocket connection
        };
        ws.send(JSON.stringify(message));
        messageInput.value = ''; // Clear input field
        // Optimistically display sent message? Or wait for echo from server?
        // displayChatMessage(userID, messageText, new Date().toISOString(), currentRoomID, true); // true for 'sent'
    }

    function displayChatMessage(senderID, content, timestamp, roomID, isSent = false) {
        if (roomID !== currentRoomID && !isSent) return; // Only display messages for the current room unless it's an optimistic send

        const messageDiv = document.createElement('div');
        messageDiv.classList.add('message');

        // Determine if message is from current user or another user
        if (senderID === userID) {
            messageDiv.classList.add('sent'); // User's own message
        } else {
            messageDiv.classList.add('received'); // Message from others
        }

        const usernameSpan = document.createElement('span');
        usernameSpan.className = 'username';
        usernameSpan.textContent = senderID; // Display UserID as username for now

        const contentSpan = document.createElement('span');
        contentSpan.className = 'content';
        contentSpan.textContent = content;

        const timestampSpan = document.createElement('span');
        timestampSpan.className = 'timestamp';
        timestampSpan.textContent = new Date(timestamp).toLocaleTimeString();

        messageDiv.appendChild(usernameSpan);
        messageDiv.appendChild(contentSpan);
        messageDiv.appendChild(timestampSpan);

        messagesArea.appendChild(messageDiv);
        messagesArea.scrollTop = messagesArea.scrollHeight; // Scroll to bottom
    }

    function displaySystemMessage(content) {
        const messageDiv = document.createElement('div');
        messageDiv.classList.add('message', 'system');
        messageDiv.textContent = content;
        messagesArea.appendChild(messageDiv);
        messagesArea.scrollTop = messagesArea.scrollHeight;
    }

    // Attempt to connect if tokens are already in localStorage (e.g., after page refresh)
    if (accessToken) {
        updateUI();
    }
});
