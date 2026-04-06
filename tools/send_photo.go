package tools

import (
	"encoding/json"
	"fmt"
	"os"
)

// PhotoSender is a callback that sends a photo to the current chat.
type PhotoSender func(sessionID, filePath, caption string) error

// SendPhoto sends a photo to the current chat session.
type SendPhoto struct {
	sender  PhotoSender
	ctx     CallContext
}

func NewSendPhoto(sender PhotoSender) *SendPhoto {
	return &SendPhoto{sender: sender}
}

func (t *SendPhoto) Name() string { return "send_photo" }
func (t *SendPhoto) Description() string {
	return "Send a photo to the current chat. Use this after generating or downloading an image to show it to the user."
}

func (t *SendPhoto) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {
				"type": "string",
				"description": "Path to the image file to send"
			},
			"caption": {
				"type": "string",
				"description": "Optional caption for the photo"
			}
		},
		"required": ["path"]
	}`)
}

func (t *SendPhoto) SetContext(ctx CallContext) {
	t.ctx = ctx
}

func (t *SendPhoto) Execute(args json.RawMessage) (string, error) {
	var params struct {
		Path    string `json:"path"`
		Caption string `json:"caption"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("parse args: %w", err)
	}

	if _, err := os.Stat(params.Path); err != nil {
		return "", fmt.Errorf("file not found: %s", params.Path)
	}

	if t.ctx.SessionID == "" {
		return "", fmt.Errorf("no active session")
	}

	if err := t.sender(t.ctx.SessionID, params.Path, params.Caption); err != nil {
		return "", fmt.Errorf("send photo: %w", err)
	}

	return fmt.Sprintf("Photo sent: %s", params.Path), nil
}
