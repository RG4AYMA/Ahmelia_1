package voiceprotocol

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Message is the channel-level envelope emitted by the voice adapter.
// Framing belongs to transport, not cognition: text may contain arbitrary
// newlines without creating additional conversational turns.
type Message struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func Encode(text string) ([]byte, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, fmt.Errorf("voice protocol: empty turn")
	}
	return json.Marshal(Message{Type: "turn", Text: text})
}

func Decode(line string) (string, error) {
	var msg Message
	if err := json.Unmarshal([]byte(line), &msg); err != nil {
		return "", fmt.Errorf("voice protocol: invalid frame: %w", err)
	}
	if msg.Type != "turn" {
		return "", fmt.Errorf("voice protocol: unsupported message type %q", msg.Type)
	}
	msg.Text = strings.TrimSpace(msg.Text)
	if msg.Text == "" {
		return "", fmt.Errorf("voice protocol: empty turn")
	}
	return msg.Text, nil
}
