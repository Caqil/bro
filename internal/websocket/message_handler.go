package websocket

import (
	"encoding/json"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"

	"bro/internal/models"
	"bro/pkg/logger"
)

// MessageHandler defines the interface for message handlers
type MessageHandler interface {
	HandleMessage(client *Client, message *WSMessage) error
	GetType() string
}

// AuthHandler handles authentication messages
type AuthHandler struct {
	hub *Hub
}

func (h *AuthHandler) GetType() string {
	return "authenticate"
}

func (h *AuthHandler) HandleMessage(client *Client, message *WSMessage) error {
	// Parse authentication data
	var authData struct {
		Token    string `json:"token"`
		DeviceID string `json:"device_id"`
		Platform string `json:"platform"`
	}

	dataBytes, err := json.Marshal(message.Data)
	if err != nil {
		return fmt.Errorf("failed to marshal auth data: %w", err)
	}

	if err := json.Unmarshal(dataBytes, &authData); err != nil {
		return fmt.Errorf("invalid auth data: %w", err)
	}

	if authData.Token == "" {
		return client.SendError("MISSING_TOKEN", "Authentication token is required", "")
	}

	// Authenticate client
	if err := h.hub.authenticateClient(client, authData.Token); err != nil {
		logger.Errorf("Authentication failed for client %s: %v", client.GetConnID(), err)
		return client.SendError("AUTHENTICATION_FAILED", "Authentication failed", err.Error())
	}

	// Update device and platform info if provided
	if authData.DeviceID != "" {
		client.deviceID = authData.DeviceID
	}
	if authData.Platform != "" {
		client.platform = authData.Platform
	}

	logger.Infof("Client %s authenticated successfully for user %s",
		client.GetConnID(), client.GetUserID().Hex())

	return nil
}

// PingHandler handles ping messages
type PingHandler struct{}

func (h *PingHandler) GetType() string {
	return "ping"
}

func (h *PingHandler) HandleMessage(client *Client, message *WSMessage) error {
	return client.SendJSON("pong", map[string]interface{}{
		"timestamp": time.Now().Unix(),
		"client_id": client.GetConnID(),
	})
}

// PresenceHandler handles presence update messages
type PresenceHandler struct {
	hub *Hub
}

func (h *PresenceHandler) GetType() string {
	return "presence"
}

func (h *PresenceHandler) HandleMessage(client *Client, message *WSMessage) error {
	if !client.IsAuthenticated() {
		return client.SendError("UNAUTHORIZED", "Authentication required", "")
	}

	// Parse presence data
	var presenceData struct {
		Status   string `json:"status"` // "online", "away", "busy", "offline"
		Activity string `json:"activity,omitempty"`
	}

	dataBytes, err := json.Marshal(message.Data)
	if err != nil {
		return fmt.Errorf("failed to marshal presence data: %w", err)
	}

	if err := json.Unmarshal(dataBytes, &presenceData); err != nil {
		return fmt.Errorf("invalid presence data: %w", err)
	}

	// Update user presence
	userID := client.GetUserID()

	// Broadcast presence change to other users
	h.hub.SendToAll("presence_update", map[string]interface{}{
		"user_id":   userID.Hex(),
		"status":    presenceData.Status,
		"activity":  presenceData.Activity,
		"timestamp": time.Now().Unix(),
	})

	// Update presence in Redis
	if h.hub.redisClient != nil {
		presenceInfo := map[string]interface{}{
			"user_id":   userID.Hex(),
			"status":    presenceData.Status,
			"activity":  presenceData.Activity,
			"device_id": client.deviceID,
			"platform":  client.platform,
			"last_seen": time.Now(),
		}
		h.hub.redisClient.SetEX(
			fmt.Sprintf("presence:%s", userID.Hex()),
			presenceInfo,
			5*time.Minute,
		)
	}

	logger.Debugf("User %s updated presence to %s", userID.Hex(), presenceData.Status)

	return client.SendJSON("presence_updated", map[string]interface{}{
		"status": "success",
	})
}

// TypingHandler handles typing indicator messages
type TypingHandler struct {
	hub *Hub
}

func (h *TypingHandler) GetType() string {
	return "typing"
}

func (h *TypingHandler) HandleMessage(client *Client, message *WSMessage) error {
	if !client.IsAuthenticated() {
		return client.SendError("UNAUTHORIZED", "Authentication required", "")
	}

	// Parse typing data
	var typingData struct {
		ChatID string `json:"chat_id"`
		Typing bool   `json:"typing"`
		Type   string `json:"type,omitempty"` // "text", "voice", etc.
	}

	dataBytes, err := json.Marshal(message.Data)
	if err != nil {
		return fmt.Errorf("failed to marshal typing data: %w", err)
	}

	if err := json.Unmarshal(dataBytes, &typingData); err != nil {
		return fmt.Errorf("invalid typing data: %w", err)
	}

	if typingData.ChatID == "" {
		return client.SendError("MISSING_CHAT_ID", "Chat ID is required", "")
	}

	chatID, err := primitive.ObjectIDFromHex(typingData.ChatID)
	if err != nil {
		return client.SendError("INVALID_CHAT_ID", "Invalid chat ID format", "")
	}

	userID := client.GetUserID()

	// Update typing state
	if typingData.Typing {
		client.SetTypingInChat(chatID)
	} else {
		client.SetTypingInChat(primitive.NilObjectID)
	}

	// Broadcast typing indicator to other chat participants
	h.hub.SendToChat(chatID, "typing_indicator", map[string]interface{}{
		"user_id":   userID.Hex(),
		"chat_id":   chatID.Hex(),
		"typing":    typingData.Typing,
		"type":      typingData.Type,
		"timestamp": time.Now().Unix(),
	})

	logger.Debugf("User %s %s typing in chat %s",
		userID.Hex(),
		func() string {
			if typingData.Typing {
				return "started"
			} else {
				return "stopped"
			}
		}(),
		chatID.Hex())

	return nil
}

// JoinChatHandler handles joining chat rooms
type JoinChatHandler struct {
	hub *Hub
}

func (h *JoinChatHandler) GetType() string {
	return "join_chat"
}

func (h *JoinChatHandler) HandleMessage(client *Client, message *WSMessage) error {
	if !client.IsAuthenticated() {
		return client.SendError("UNAUTHORIZED", "Authentication required", "")
	}

	// Parse join data
	var joinData struct {
		ChatID string `json:"chat_id"`
	}

	dataBytes, err := json.Marshal(message.Data)
	if err != nil {
		return fmt.Errorf("failed to marshal join data: %w", err)
	}

	if err := json.Unmarshal(dataBytes, &joinData); err != nil {
		return fmt.Errorf("invalid join data: %w", err)
	}

	if joinData.ChatID == "" {
		return client.SendError("MISSING_CHAT_ID", "Chat ID is required", "")
	}

	chatID, err := primitive.ObjectIDFromHex(joinData.ChatID)
	if err != nil {
		return client.SendError("INVALID_CHAT_ID", "Invalid chat ID format", "")
	}

	// TODO: Verify user is participant of this chat (check database)
	// For now, we'll allow joining any chat

	// Join chat room
	h.hub.JoinChat(client, chatID)

	userID := client.GetUserID()

	// Notify other chat participants about user joining
	h.hub.SendToChat(chatID, "user_joined_chat", map[string]interface{}{
		"user_id":   userID.Hex(),
		"chat_id":   chatID.Hex(),
		"timestamp": time.Now().Unix(),
	})

	logger.Debugf("User %s joined chat %s", userID.Hex(), chatID.Hex())

	return client.SendJSON("chat_joined", map[string]interface{}{
		"chat_id": chatID.Hex(),
		"status":  "success",
	})
}

// LeaveChatHandler handles leaving chat rooms
type LeaveChatHandler struct {
	hub *Hub
}

func (h *LeaveChatHandler) GetType() string {
	return "leave_chat"
}

func (h *LeaveChatHandler) HandleMessage(client *Client, message *WSMessage) error {
	if !client.IsAuthenticated() {
		return client.SendError("UNAUTHORIZED", "Authentication required", "")
	}

	// Parse leave data
	var leaveData struct {
		ChatID string `json:"chat_id"`
	}

	dataBytes, err := json.Marshal(message.Data)
	if err != nil {
		return fmt.Errorf("failed to marshal leave data: %w", err)
	}

	if err := json.Unmarshal(dataBytes, &leaveData); err != nil {
		return fmt.Errorf("invalid leave data: %w", err)
	}

	if leaveData.ChatID == "" {
		return client.SendError("MISSING_CHAT_ID", "Chat ID is required", "")
	}

	chatID, err := primitive.ObjectIDFromHex(leaveData.ChatID)
	if err != nil {
		return client.SendError("INVALID_CHAT_ID", "Invalid chat ID format", "")
	}

	// Leave chat room
	h.hub.LeaveChat(client, chatID)

	userID := client.GetUserID()

	// Notify other chat participants about user leaving
	h.hub.SendToChat(chatID, "user_left_chat", map[string]interface{}{
		"user_id":   userID.Hex(),
		"chat_id":   chatID.Hex(),
		"timestamp": time.Now().Unix(),
	})

	logger.Debugf("User %s left chat %s", userID.Hex(), chatID.Hex())

	return client.SendJSON("chat_left", map[string]interface{}{
		"chat_id": chatID.Hex(),
		"status":  "success",
	})
}

// CallSignalingHandler handles WebRTC call signaling - FIXED to route to WebRTC signaling server
type CallSignalingHandler struct {
	hub             *Hub
	signalingServer interface{} // This should be *webrtc.SignalingServer but avoiding circular import
}

func (h *CallSignalingHandler) GetType() string {
	return "call_signal"
}

func (h *CallSignalingHandler) HandleMessage(client *Client, message *WSMessage) error {
	if !client.IsAuthenticated() {
		return client.SendError("UNAUTHORIZED", "Authentication required", "")
	}

	// Route to WebRTC signaling server if available
	if h.signalingServer != nil {
		// Use interface{} to avoid circular imports
		// The hub should inject the signaling server reference
		if signalingHandler, ok := h.signalingServer.(interface {
			HandleSignalingMessage(client *Client, message *WSMessage) error
		}); ok {
			return signalingHandler.HandleSignalingMessage(client, message)
		}
	}

	// Fallback: Parse signaling data and forward manually
	var signalData struct {
		CallID   string      `json:"call_id"`
		TargetID string      `json:"target_id,omitempty"`
		Type     string      `json:"type"` // "offer", "answer", "ice-candidate", "end", "call_initiate", "call_response"
		Signal   interface{} `json:"signal,omitempty"`
		Data     interface{} `json:"data,omitempty"`
		Metadata interface{} `json:"metadata,omitempty"`
	}

	dataBytes, err := json.Marshal(message.Data)
	if err != nil {
		return fmt.Errorf("failed to marshal signal data: %w", err)
	}

	if err := json.Unmarshal(dataBytes, &signalData); err != nil {
		return fmt.Errorf("invalid signal data: %w", err)
	}

	if signalData.Type == "" {
		return client.SendError("MISSING_SIGNAL_TYPE", "Signal type is required", "")
	}

	userID := client.GetUserID()

	// Handle different signal types
	switch signalData.Type {
	case "call_initiate":
		// This should be handled by the WebRTC signaling server
		return h.handleCallInitiate(client, signalData)

	case "call_response":
		// This should be handled by the WebRTC signaling server
		return h.handleCallResponse(client, signalData)

	case "offer", "answer", "ice-candidate":
		// Forward signaling to target user if specified
		if signalData.TargetID == "" {
			return client.SendError("MISSING_TARGET_ID", "Target user ID is required for signaling", "")
		}

		targetID, err := primitive.ObjectIDFromHex(signalData.TargetID)
		if err != nil {
			return client.SendError("INVALID_TARGET_ID", "Invalid target user ID format", "")
		}

		// Forward signal to target user with consistent format
		h.hub.SendToUser(targetID, "call_signal", map[string]interface{}{
			"type":      signalData.Type,
			"call_id":   signalData.CallID,
			"from_user": userID.Hex(),
			"signal":    signalData.Signal,
			"data":      signalData.Data,
			"metadata":  signalData.Metadata,
			"timestamp": time.Now().Unix(),
		})

		logger.Debugf("Forwarded %s signal from %s to %s for call %s",
			signalData.Type, userID.Hex(), targetID.Hex(), signalData.CallID)

	case "end":
		// Handle call end
		if signalData.CallID != "" {
			callID, err := primitive.ObjectIDFromHex(signalData.CallID)
			if err == nil {
				client.SetActiveCall(primitive.NilObjectID)

				// Notify other participants about call end
				h.hub.SendToChat(callID, "call_ended", map[string]interface{}{
					"call_id":   signalData.CallID,
					"ended_by":  userID.Hex(),
					"timestamp": time.Now().Unix(),
				})
			}
		}

		logger.Debugf("User %s ended call %s", userID.Hex(), signalData.CallID)

	default:
		return client.SendError("UNKNOWN_SIGNAL_TYPE", "Unknown signal type", signalData.Type)
	}

	return client.SendJSON("signal_sent", map[string]interface{}{
		"call_id": signalData.CallID,
		"type":    signalData.Type,
		"status":  "success",
	})
}

// handleCallInitiate handles call initiation (fallback)
func (h *CallSignalingHandler) handleCallInitiate(client *Client, signalData struct {
	CallID   string      `json:"call_id"`
	TargetID string      `json:"target_id,omitempty"`
	Type     string      `json:"type"`
	Signal   interface{} `json:"signal,omitempty"`
	Data     interface{} `json:"data,omitempty"`
	Metadata interface{} `json:"metadata,omitempty"`
}) error {
	// This should be handled by WebRTC signaling server
	// For now, just forward to participants
	userID := client.GetUserID()

	if dataMap, ok := signalData.Data.(map[string]interface{}); ok {
		if participants, ok := dataMap["participants"].([]interface{}); ok {
			for _, p := range participants {
				if participantID, ok := p.(string); ok {
					if targetID, err := primitive.ObjectIDFromHex(participantID); err == nil {
						h.hub.SendToUser(targetID, "incoming_call", map[string]interface{}{
							"call_id":   signalData.CallID,
							"from_user": userID.Hex(),
							"call_type": dataMap["type"],
							"video":     dataMap["video_enabled"],
							"timestamp": time.Now().Unix(),
						})
					}
				}
			}
		}
	}

	return nil
}

// handleCallResponse handles call response (fallback)
func (h *CallSignalingHandler) handleCallResponse(client *Client, signalData struct {
	CallID   string      `json:"call_id"`
	TargetID string      `json:"target_id,omitempty"`
	Type     string      `json:"type"`
	Signal   interface{} `json:"signal,omitempty"`
	Data     interface{} `json:"data,omitempty"`
	Metadata interface{} `json:"metadata,omitempty"`
}) error {
	// This should be handled by WebRTC signaling server
	// For now, just forward response
	userID := client.GetUserID()

	if signalData.TargetID != "" {
		if targetID, err := primitive.ObjectIDFromHex(signalData.TargetID); err == nil {
			h.hub.SendToUser(targetID, "call_response", map[string]interface{}{
				"call_id":   signalData.CallID,
				"from_user": userID.Hex(),
				"response":  signalData.Data,
				"timestamp": time.Now().Unix(),
			})
		}
	}

	return nil
}

// SetSignalingServer sets the WebRTC signaling server reference
func (h *CallSignalingHandler) SetSignalingServer(server interface{}) {
	h.signalingServer = server
}

// MessageHandler for handling chat messages (optional - might be handled via HTTP)
type ChatMessageHandler struct {
	hub *Hub
}

func (h *ChatMessageHandler) GetType() string {
	return "chat_message"
}

func (h *ChatMessageHandler) HandleMessage(client *Client, message *WSMessage) error {
	if !client.IsAuthenticated() {
		return client.SendError("UNAUTHORIZED", "Authentication required", "")
	}

	// Parse message data
	var msgData struct {
		ChatID  string             `json:"chat_id"`
		Type    models.MessageType `json:"type"`
		Content string             `json:"content"`
		ReplyTo string             `json:"reply_to,omitempty"`
	}

	dataBytes, err := json.Marshal(message.Data)
	if err != nil {
		return fmt.Errorf("failed to marshal message data: %w", err)
	}

	if err := json.Unmarshal(dataBytes, &msgData); err != nil {
		return fmt.Errorf("invalid message data: %w", err)
	}

	if msgData.ChatID == "" {
		return client.SendError("MISSING_CHAT_ID", "Chat ID is required", "")
	}

	if msgData.Content == "" {
		return client.SendError("MISSING_CONTENT", "Message content is required", "")
	}

	chatID, err := primitive.ObjectIDFromHex(msgData.ChatID)
	if err != nil {
		return client.SendError("INVALID_CHAT_ID", "Invalid chat ID format", "")
	}

	userID := client.GetUserID()

	// TODO: Save message to database via ChatService
	// For now, just broadcast to chat participants

	// Create message response
	messageResponse := map[string]interface{}{
		"message_id": primitive.NewObjectID().Hex(),
		"chat_id":    chatID.Hex(),
		"sender_id":  userID.Hex(),
		"type":       msgData.Type,
		"content":    msgData.Content,
		"reply_to":   msgData.ReplyTo,
		"timestamp":  time.Now().Unix(),
		"status":     "sent",
	}

	// Broadcast to chat participants
	h.hub.SendToChat(chatID, "new_message", messageResponse)

	logger.Debugf("User %s sent message to chat %s", userID.Hex(), chatID.Hex())

	return client.SendJSON("message_sent", map[string]interface{}{
		"message_id": messageResponse["message_id"],
		"status":     "success",
	})
}

// StatusHandler handles user status updates
type StatusHandler struct {
	hub *Hub
}

func (h *StatusHandler) GetType() string {
	return "status_update"
}

func (h *StatusHandler) HandleMessage(client *Client, message *WSMessage) error {
	if !client.IsAuthenticated() {
		return client.SendError("UNAUTHORIZED", "Authentication required", "")
	}

	// Parse status data
	var statusData struct {
		Text      string `json:"text"`
		Emoji     string `json:"emoji"`
		ExpiresIn int64  `json:"expires_in,omitempty"` // seconds from now
	}

	dataBytes, err := json.Marshal(message.Data)
	if err != nil {
		return fmt.Errorf("failed to marshal status data: %w", err)
	}

	if err := json.Unmarshal(dataBytes, &statusData); err != nil {
		return fmt.Errorf("invalid status data: %w", err)
	}

	userID := client.GetUserID()

	// TODO: Update user status in database
	// For now, just broadcast to contacts

	// Calculate expiry time
	var expiresAt *time.Time
	if statusData.ExpiresIn > 0 {
		expiry := time.Now().Add(time.Duration(statusData.ExpiresIn) * time.Second)
		expiresAt = &expiry
	}

	// Broadcast status update
	h.hub.SendToAll("user_status_update", map[string]interface{}{
		"user_id":    userID.Hex(),
		"text":       statusData.Text,
		"emoji":      statusData.Emoji,
		"expires_at": expiresAt,
		"timestamp":  time.Now().Unix(),
	})

	logger.Debugf("User %s updated status: %s %s", userID.Hex(), statusData.Emoji, statusData.Text)

	return client.SendJSON("status_updated", map[string]interface{}{
		"status": "success",
	})
}

// ReactionHandler handles message reactions
type ReactionHandler struct {
	hub *Hub
}

func (h *ReactionHandler) GetType() string {
	return "reaction"
}

func (h *ReactionHandler) HandleMessage(client *Client, message *WSMessage) error {
	if !client.IsAuthenticated() {
		return client.SendError("UNAUTHORIZED", "Authentication required", "")
	}

	// Parse reaction data
	var reactionData struct {
		MessageID string `json:"message_id"`
		ChatID    string `json:"chat_id"`
		Emoji     string `json:"emoji"`
		Action    string `json:"action"` // "add" or "remove"
	}

	dataBytes, err := json.Marshal(message.Data)
	if err != nil {
		return fmt.Errorf("failed to marshal reaction data: %w", err)
	}

	if err := json.Unmarshal(dataBytes, &reactionData); err != nil {
		return fmt.Errorf("invalid reaction data: %w", err)
	}

	if reactionData.MessageID == "" {
		return client.SendError("MISSING_MESSAGE_ID", "Message ID is required", "")
	}

	if reactionData.ChatID == "" {
		return client.SendError("MISSING_CHAT_ID", "Chat ID is required", "")
	}

	if reactionData.Emoji == "" {
		return client.SendError("MISSING_EMOJI", "Emoji is required", "")
	}

	if reactionData.Action != "add" && reactionData.Action != "remove" {
		return client.SendError("INVALID_ACTION", "Action must be 'add' or 'remove'", "")
	}

	chatID, err := primitive.ObjectIDFromHex(reactionData.ChatID)
	if err != nil {
		return client.SendError("INVALID_CHAT_ID", "Invalid chat ID format", "")
	}

	messageID, err := primitive.ObjectIDFromHex(reactionData.MessageID)
	if err != nil {
		return client.SendError("INVALID_MESSAGE_ID", "Invalid message ID format", "")
	}

	userID := client.GetUserID()

	// TODO: Update message reactions in database
	// For now, just broadcast to chat participants

	// Broadcast reaction update
	h.hub.SendToChat(chatID, "message_reaction", map[string]interface{}{
		"message_id": messageID.Hex(),
		"chat_id":    chatID.Hex(),
		"user_id":    userID.Hex(),
		"emoji":      reactionData.Emoji,
		"action":     reactionData.Action,
		"timestamp":  time.Now().Unix(),
	})

	logger.Debugf("User %s %s reaction %s on message %s",
		userID.Hex(), reactionData.Action, reactionData.Emoji, messageID.Hex())

	return client.SendJSON("reaction_updated", map[string]interface{}{
		"message_id": messageID.Hex(),
		"action":     reactionData.Action,
		"status":     "success",
	})
}

// ReadReceiptHandler handles message read receipts
type ReadReceiptHandler struct {
	hub *Hub
}

func (h *ReadReceiptHandler) GetType() string {
	return "read_receipt"
}

func (h *ReadReceiptHandler) HandleMessage(client *Client, message *WSMessage) error {
	if !client.IsAuthenticated() {
		return client.SendError("UNAUTHORIZED", "Authentication required", "")
	}

	// Parse read receipt data
	var receiptData struct {
		MessageID string `json:"message_id"`
		ChatID    string `json:"chat_id"`
	}

	dataBytes, err := json.Marshal(message.Data)
	if err != nil {
		return fmt.Errorf("failed to marshal receipt data: %w", err)
	}

	if err := json.Unmarshal(dataBytes, &receiptData); err != nil {
		return fmt.Errorf("invalid receipt data: %w", err)
	}

	if receiptData.MessageID == "" {
		return client.SendError("MISSING_MESSAGE_ID", "Message ID is required", "")
	}

	if receiptData.ChatID == "" {
		return client.SendError("MISSING_CHAT_ID", "Chat ID is required", "")
	}

	chatID, err := primitive.ObjectIDFromHex(receiptData.ChatID)
	if err != nil {
		return client.SendError("INVALID_CHAT_ID", "Invalid chat ID format", "")
	}

	messageID, err := primitive.ObjectIDFromHex(receiptData.MessageID)
	if err != nil {
		return client.SendError("INVALID_MESSAGE_ID", "Invalid message ID format", "")
	}

	userID := client.GetUserID()

	// TODO: Update message read status in database
	// For now, just broadcast to chat participants

	// Broadcast read receipt
	h.hub.SendToChat(chatID, "message_read", map[string]interface{}{
		"message_id": messageID.Hex(),
		"chat_id":    chatID.Hex(),
		"user_id":    userID.Hex(),
		"read_at":    time.Now().Unix(),
	})

	logger.Debugf("User %s read message %s in chat %s",
		userID.Hex(), messageID.Hex(), chatID.Hex())

	return nil // No response needed for read receipts
}
