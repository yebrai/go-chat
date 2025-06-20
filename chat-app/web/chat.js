document.addEventListener('DOMContentLoaded', () => {
    // Views
    const setupView = document.getElementById('setup-view');
    const chatView = document.getElementById('chat-view');

    // Setup Form Elements
    const joinForm = document.getElementById('join-form');
    const usernameInput = document.getElementById('username-input');
    const roomIdSetupInput = document.getElementById('room-id-setup-input');

    // Chat View Elements - Header
    const displayUsername = document.getElementById('display-username');
    const displayRoomId = document.getElementById('display-room-id');
    const globalUserCountSpan = document.getElementById('global-user-count');
    const roomUserCountSpan = document.getElementById('room-user-count');
    const connectionStatusSpan = document.getElementById('connection-status');

    // Chat View Elements - Sidebar
    const userListUl = document.getElementById('user-list');
    const roomListExampleUl = document.getElementById('room-list-example'); // For example room switching
    const newRoomIdInput = document.getElementById('new-room-id-input');
    const switchRoomBtn = document.getElementById('switch-room-btn');


    // Chat View Elements - Chat Area
    const messageArea = document.getElementById('message-area');
    const typingIndicatorDiv = document.getElementById('typing-indicator');

    // Chat View Elements - Footer
    const messageForm = document.getElementById('message-form');
    const messageInput = document.getElementById('message-input');

    // Client State
    let ws = null;
    let currentUsername = '';
    let currentRoomID = '';
    let isTyping = false;
    let typingTimer = null;
    let reconnectAttempts = 0;
    const maxReconnectAttempts = 5;
    const baseReconnectDelay = 1000; // 1 second

    // MessageType constants (mirroring backend)
    const MessageType = {
        Text: "text_message",
        UserJoined: "user_joined",
        UserLeft: "user_left",
        RoomStatsUpdate: "room_stats",
        RecentMessages: "recent_messages",
        UserListUpdate: "user_list",
        GlobalUserCountUpdate: "global_user_count",
        Error: "error_message",
        JoinRoom: "join_room", // Client to Server
        LeaveRoom: "leave_room", // Client to Server (not explicitly used in this UI version yet)
        UserTyping: "user_typing", // Client to Server & Server to Client
        RequestStats: "request_stats" // Client to Server
    };

    // --- Initialization ---
    function initApp() {
        setupView.style.display = 'block';
        chatView.style.display = 'none';
        joinForm.addEventListener('submit', handleJoinChat);
        messageForm.addEventListener('submit', handleSendMessage);
        messageInput.addEventListener('input', handleTyping);
        switchRoomBtn.addEventListener('click', handleSwitchRoom);

        // Example room list click handling
        roomListExampleUl.addEventListener('click', (event) => {
            if (event.target.tagName === 'A' && event.target.dataset.roomid) {
                event.preventDefault();
                const newRoom = event.target.dataset.roomid;
                if (newRoom !== currentRoomID) {
                    newRoomIdInput.value = newRoom; // Populate input for clarity
                    handleSwitchRoom();
                }
            }
        });
    }

    // --- Event Handlers ---
    function handleJoinChat(event) {
        event.preventDefault();
        const username = usernameInput.value.trim();
        const roomID = roomIdSetupInput.value.trim();

        if (!username || !roomID) {
            alert('Username and Room ID are required.');
            return;
        }

        currentUsername = username;
        currentRoomID = roomID; // Set initial room

        displayUsername.textContent = currentUsername;
        displayRoomId.textContent = currentRoomID;

        setupView.style.display = 'none';
        chatView.style.display = 'flex'; // Use flex as per new CSS

        connectWebSocket(currentUsername, currentRoomID);
    }

    function handleSendMessage(event) {
        event.preventDefault();
        const text = messageInput.value.trim();
        if (!text || !ws || ws.readyState !== WebSocket.OPEN) {
            return;
        }

        // Stop typing indicator immediately
        if (isTyping) {
            sendTypingMessage(false); // Send 'stop'
            clearTimeout(typingTimer);
            isTyping = false;
        }

        if (text.startsWith('/stats')) {
            const parts = text.split(' ');
            const targetRoomID = parts[1] ? parts[1].trim() : currentRoomID;
            ws.send(JSON.stringify({
                type: MessageType.RequestStats,
                roomID: targetRoomID,
                username: currentUsername
            }));
        } else {
            ws.send(JSON.stringify({
                type: MessageType.Text,
                content: text,
                roomID: currentRoomID,
                username: currentUsername // Client sends its username, server can verify/override
            }));
        }
        messageInput.value = '';
    }

    function handleTyping() {
        if (!ws || ws.readyState !== WebSocket.OPEN) return;

        if (!isTyping) {
            sendTypingMessage(true); // Send 'start'
            isTyping = true;
        }
        clearTimeout(typingTimer);
        typingTimer = setTimeout(() => {
            sendTypingMessage(false); // Send 'stop'
            isTyping = false;
        }, 2000); // 2 seconds debounce
    }

    function sendTypingMessage(isStarting) {
        if (!ws || ws.readyState !== WebSocket.OPEN || !currentRoomID) return;
        ws.send(JSON.stringify({
            type: MessageType.UserTyping,
            roomID: currentRoomID,
            username: currentUsername,
            content: isStarting ? 'start' : 'stop'
        }));
    }

    function handleSwitchRoom() {
        const newRoom = newRoomIdInput.value.trim();
        if (!newRoom || newRoom === currentRoomID) {
            if (!newRoom) alert("Please enter a Room ID to switch to.");
            else alert(`You are already in room: ${newRoom}`);
            return;
        }

        if (!ws || ws.readyState !== WebSocket.OPEN) {
            alert("WebSocket is not connected.");
            return;
        }

        // Send a join_room message. Server will handle leaving the old room.
        ws.send(JSON.stringify({
            type: MessageType.JoinRoom,
            // RoomID in content for join_room as per previous backend thoughts for payload
            // Or better, use Data field if backend expects structured payload.
            // Let's assume backend hub.go expects msg.Content to be JSON for JoinRoomData
            content: JSON.stringify({ roomID: newRoom }),
            username: currentUsername // Username initiating the join
        }));

        // UI updates are now mostly driven by server messages (UserLeft from old room, UserJoined to new room etc.)
        // However, we can optimistically update currentRoomID display and clear old messages.
        currentRoomID = newRoom; // Optimistic update
        displayRoomId.textContent = currentRoomID;
        messageArea.innerHTML = ''; // Clear messages from old room
        userListUl.innerHTML = ''; // Clear old user list
        roomUserCountSpan.textContent = '0'; // Reset room user count
        typingIndicatorDiv.textContent = ''; // Clear typing indicator
        displaySystemMessage(`Attempting to join room: ${newRoom}...`);
    }


    // --- WebSocket Logic ---
    function connectWebSocket(username, roomID) {
        if (ws && ws.readyState === WebSocket.OPEN) {
            ws.close();
        }

        const wsUrl = `ws://${window.location.host}/ws?username=${encodeURIComponent(username)}&roomID=${encodeURIComponent(roomID)}`;
        ws = new WebSocket(wsUrl);
        updateConnectionStatus('reconnecting', 'Connecting...');

        ws.onopen = () => {
            reconnectAttempts = 0; // Reset reconnect attempts on successful connection
            updateConnectionStatus('connected', 'Connected');
            console.log(`WebSocket connected for user ${username} in room ${roomID}`);
            // Initial join is handled by server based on query params.
            // No explicit "join_room" message needed here for the *initial* room.
        };

        ws.onmessage = (event) => {
            const msg = JSON.parse(event.data);
            console.log("WS Message Received:", msg);

            switch (msg.type) {
                case MessageType.Text:
                    displayMessage(msg, false);
                    break;
                case MessageType.UserJoined:
                case MessageType.UserLeft:
                    displaySystemMessage(msg.content); // Content usually like "UserX joined/left"
                    // User list and room stats are updated via their specific messages
                    break;
                case MessageType.RecentMessages:
                    if (msg.data && msg.data.messages) {
                        msg.data.messages.forEach(mJson => {
                            try {
                                const historicalMsg = JSON.parse(mJson); // Messages are stored as JSON strings
                                displayMessage(historicalMsg, true);
                            } catch (e) { console.error("Error parsing historical message:", e, mJson); }
                        });
                         // Add a marker for historical messages
                        if (msg.data.messages.length > 0) {
                            displaySystemMessage("--- Previous messages loaded ---");
                        } else {
                            displaySystemMessage("No previous messages in this room.");
                        }
                    }
                    break;
                case MessageType.UserListUpdate:
                    if (msg.data) updateUserList(msg.data);
                    break;
                case MessageType.GlobalUserCountUpdate:
                    if (msg.data) updateGlobalUserCount(msg.data);
                    break;
                case MessageType.RoomStatsUpdate:
                     if (msg.data && msg.roomID === currentRoomID) { // Ensure stats are for current room
                        updateRoomStatsDisplay(msg.data);
                    }
                    break;
                case MessageType.UserTyping:
                    showTypingIndicator(msg.username, msg.content === 'start');
                    break;
                case MessageType.Error:
                    displaySystemMessage(`Error from server: ${msg.content || (msg.data && msg.data.message)}`, true);
                    break;
                default:
                    console.warn("Received unknown message type:", msg.type);
                    displaySystemMessage(`Unknown event: ${msg.type} - ${msg.content || ''}`);
            }
        };

        ws.onerror = (error) => {
            console.error('WebSocket Error:', error);
            updateConnectionStatus('disconnected', 'Connection Error');
            // Reconnect logic is handled in onclose
        };

        ws.onclose = (event) => {
            const reason = event.reason || (event.wasClean ? "Clean close" : "Connection died");
            updateConnectionStatus('disconnected', `Disconnected: ${reason} (Code: ${event.code})`);
            ws = null; // Important to nullify ws object

            if (reconnectAttempts < maxReconnectAttempts && event.code !== 1000) { // Don't retry on normal close (1000)
                const delay = Math.pow(2, reconnectAttempts) * baseReconnectDelay;
                reconnectAttempts++;
                console.log(`Attempting to reconnect in ${delay / 1000}s (attempt ${reconnectAttempts}/${maxReconnectAttempts})`);
                updateConnectionStatus('reconnecting', `Reconnecting (attempt ${reconnectAttempts})...`);
                setTimeout(() => connectWebSocket(currentUsername, currentRoomID), delay);
            } else if (reconnectAttempts >= maxReconnectAttempts) {
                console.error("Max reconnect attempts reached. Please manually rejoin.");
                updateConnectionStatus('disconnected', 'Failed to reconnect. Please rejoin.');
            }
        };
    }

    // --- UI Update Functions ---
    function displayMessage(msg, isHistory = false) {
        const item = document.createElement('div');
        item.classList.add('message');
        if (msg.system) {
            item.classList.add('system');
            item.textContent = msg.content;
        } else {
            item.classList.add(msg.username === currentUsername ? 'mine' : 'other');
            item.innerHTML = `<strong>${msg.username}</strong>: ${msg.content}
                              <span class="timestamp">${new Date(msg.timestamp).toLocaleTimeString()}</span>`;
        }

        if (!isHistory) {
            item.classList.add('new'); // For slide-in animation
        }

        messageArea.appendChild(item);
        messageArea.scrollTop = messageArea.scrollHeight;
    }

    function displaySystemMessage(content, isError = false) {
        const item = document.createElement('div');
        item.classList.add('message', 'system');
        if (isError) item.style.color = 'red';
        item.textContent = content;
        messageArea.appendChild(item);
        messageArea.scrollTop = messageArea.scrollHeight;
    }

    function updateUserList(userListPayload) { // { roomID: "...", users: ["user1", "user2"] }
        if (userListPayload.roomID !== currentRoomID) return; // Update only if for current room

        userListUl.innerHTML = ''; // Clear existing list
        userListPayload.users.forEach(user => {
            const li = document.createElement('li');
            li.textContent = user;
            userListUl.appendChild(li);
        });
        roomUserCountSpan.textContent = userListPayload.users.length;
        animateCountUpdate(roomUserCountSpan);
    }

    function updateGlobalUserCount(payload) { // { count: X }
        globalUserCountSpan.textContent = payload.count;
        animateCountUpdate(globalUserCountSpan);
    }

    function updateRoomStatsDisplay(statsPayload) { // { roomID, active_users, message_count }
        if (statsPayload.roomID === currentRoomID) {
            roomUserCountSpan.textContent = statsPayload.active_users;
            // Could also display message_count somewhere if desired
            animateCountUpdate(roomUserCountSpan);
        }
    }

    function updateConnectionStatus(statusClass, statusText) {
        connectionStatusSpan.className = ''; // Clear existing classes
        connectionStatusSpan.classList.add(statusClass);
        connectionStatusSpan.textContent = statusText;
    }

    const activeTypers = new Map(); // username -> timerID
    function showTypingIndicator(typingUsername, isStarting) {
        if (typingUsername === currentUsername) return; // Don't show own typing

        const indicatorText = `${typingUsername} is typing...`;

        if (isStarting) {
            if (!activeTypers.has(typingUsername)) {
                const timerID = setTimeout(() => {
                    // If timer expires, assume user stopped typing
                    if (activeTypers.get(typingUsername) === timerID) { // Check if it's the same timer
                        activeTypers.delete(typingUsername);
                        updateTypingIndicatorText();
                    }
                }, 3000); // User considered stopped typing after 3s of no 'start' message
                activeTypers.set(typingUsername, timerID);
            } else {
                // Refresh existing timer
                clearTimeout(activeTypers.get(typingUsername));
                 const timerID = setTimeout(() => {
                    if (activeTypers.get(typingUsername) === timerID) {
                        activeTypers.delete(typingUsername);
                        updateTypingIndicatorText();
                    }
                }, 3000);
                activeTypers.set(typingUsername, timerID);
            }
        } else { // isStopping
            clearTimeout(activeTypers.get(typingUsername));
            activeTypers.delete(typingUsername);
        }
        updateTypingIndicatorText();
    }

    function updateTypingIndicatorText() {
        const typers = Array.from(activeTypers.keys());
        if (typers.length === 0) {
            typingIndicatorDiv.textContent = '';
            typingIndicatorDiv.classList.remove('user-typing');
        } else if (typers.length === 1) {
            typingIndicatorDiv.textContent = `${typers[0]} is typing...`;
            typingIndicatorDiv.classList.add('user-typing');
        } else if (typers.length === 2) {
            typingIndicatorDiv.textContent = `${typers[0]} and ${typers[1]} are typing...`;
            typingIndicatorDiv.classList.add('user-typing');
        } else {
            typingIndicatorDiv.textContent = `Multiple users are typing...`;
            typingIndicatorDiv.classList.add('user-typing');
        }
    }

    function animateCountUpdate(element) {
        element.classList.add('count-update');
        setTimeout(() => element.classList.remove('count-update'), 500); // Duration of animation
    }

    // --- Start the app ---
    initApp();
});
