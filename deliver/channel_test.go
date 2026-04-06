package deliver

import (
	"testing"
)

func TestIncomingMessage_Fields(t *testing.T) {
	msg := IncomingMessage{
		Text:      "Hello",
		SessionID: "session-123",
		UserID:    "user-456",
		UserName:  "Alice",
		Channel:   "telegram",
		ImageDesc: "a photo of a cat",
	}

	if msg.Text != "Hello" {
		t.Errorf("Text = %q, want %q", msg.Text, "Hello")
	}
	if msg.SessionID != "session-123" {
		t.Errorf("SessionID = %q, want %q", msg.SessionID, "session-123")
	}
	if msg.UserID != "user-456" {
		t.Errorf("UserID = %q, want %q", msg.UserID, "user-456")
	}
	if msg.UserName != "Alice" {
		t.Errorf("UserName = %q, want %q", msg.UserName, "Alice")
	}
	if msg.Channel != "telegram" {
		t.Errorf("Channel = %q, want %q", msg.Channel, "telegram")
	}
	if msg.ImageDesc != "a photo of a cat" {
		t.Errorf("ImageDesc = %q, want %q", msg.ImageDesc, "a photo of a cat")
	}
}

func TestOutgoingMessage_Fields(t *testing.T) {
	msg := OutgoingMessage{
		Text:      "Hi there!",
		SessionID: "session-123",
		Channel:   "telegram",
	}

	if msg.Text != "Hi there!" {
		t.Errorf("Text = %q, want %q", msg.Text, "Hi there!")
	}
	if msg.SessionID != "session-123" {
		t.Errorf("SessionID = %q, want %q", msg.SessionID, "session-123")
	}
	if msg.Channel != "telegram" {
		t.Errorf("Channel = %q, want %q", msg.Channel, "telegram")
	}
}

func TestCommandResult_Fields(t *testing.T) {
	result := CommandResult{
		Text: "Command executed",
		OK:   true,
		Buttons: [][]InlineButton{
			{{Text: "OK", CallbackData: "ok"}},
			{{Text: "Cancel", CallbackData: "cancel"}},
		},
	}

	if result.Text != "Command executed" {
		t.Errorf("Text = %q, want %q", result.Text, "Command executed")
	}
	if !result.OK {
		t.Error("OK should be true")
	}
	if len(result.Buttons) != 2 {
		t.Errorf("Buttons rows = %d, want 2", len(result.Buttons))
	}
	if result.Buttons[0][0].Text != "OK" {
		t.Errorf("First button text = %q, want %q", result.Buttons[0][0].Text, "OK")
	}
}

func TestInlineButton_Fields(t *testing.T) {
	btn := InlineButton{
		Text:         "Click me",
		CallbackData: "btn_click",
	}

	if btn.Text != "Click me" {
		t.Errorf("Text = %q, want %q", btn.Text, "Click me")
	}
	if btn.CallbackData != "btn_click" {
		t.Errorf("CallbackData = %q, want %q", btn.CallbackData, "btn_click")
	}
}

func TestHandler(t *testing.T) {
	handler := Handler(func(msg IncomingMessage) OutgoingMessage {
		return OutgoingMessage{
			Text:      "Echo: " + msg.Text,
			SessionID: msg.SessionID,
			Channel:   msg.Channel,
		}
	})

	msg := IncomingMessage{
		Text:      "Hello",
		SessionID: "session-1",
		Channel:   "telegram",
	}

	result := handler(msg)

	if result.Text != "Echo: Hello" {
		t.Errorf("result.Text = %q, want %q", result.Text, "Echo: Hello")
	}
	if result.SessionID != "session-1" {
		t.Errorf("result.SessionID = %q, want %q", result.SessionID, "session-1")
	}
}

func TestCommandHandler(t *testing.T) {
	handler := CommandHandler(func(cmd, sessionID string) *CommandResult {
		if cmd == "/ping" {
			return &CommandResult{Text: "Pong!", OK: true}
		}
		return nil
	})

	// Test known command
	result := handler("/ping", "session-1")
	if result == nil {
		t.Fatal("result should not be nil for /ping")
	}
	if result.Text != "Pong!" {
		t.Errorf("result.Text = %q, want %q", result.Text, "Pong!")
	}

	// Test unknown command
	result = handler("/unknown", "session-1")
	if result != nil {
		t.Error("result should be nil for unknown command")
	}
}

func TestCallbackHandler(t *testing.T) {
	handler := CallbackHandler(func(data, sessionID string) *CommandResult {
		if data == "confirm" {
			return &CommandResult{
				Text: "Confirmed!",
				OK:   true,
			}
		}
		return &CommandResult{
			Text: "Unknown action",
			OK:   false,
		}
	})

	result := handler("confirm", "session-1")
	if result == nil {
		t.Fatal("result should not be nil")
	}
	if result.Text != "Confirmed!" {
		t.Errorf("result.Text = %q, want %q", result.Text, "Confirmed!")
	}
	if !result.OK {
		t.Error("result.OK should be true")
	}

	result = handler("unknown", "session-1")
	if result == nil {
		t.Fatal("result should not be nil")
	}
	if result.OK {
		t.Error("result.OK should be false for unknown action")
	}
}
