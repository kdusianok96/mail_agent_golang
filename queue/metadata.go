package queue

import (
	"encoding/json"
	"os"
	"time"
)

// MailMetadata holds information about a queued email message.
type MailMetadata struct {
	MessageID       string    `json:"messageId"`       // Unique ID for the message (e.g., generated filename base)
	Sender          string    `json:"sender"`
	Recipients      []string  `json:"recipients"`
	ReceivedTime    time.Time `json:"receivedTime"`
	NextAttemptTime time.Time `json:"nextAttemptTime"` // Initially same as ReceivedTime
	AttemptCount    int       `json:"attemptCount"`    // Initially 0
	LastAttemptTime time.Time `json:"lastAttemptTime"` // Initially zero value
	LastError       string    `json:"lastError,omitempty"`
	// Hostname field from delivery.Email could be added here if needed for EHLO in delivery
}

// LoadMetadata reads a JSON file from filePath and unmarshals it into a MailMetadata struct.
func LoadMetadata(filePath string) (*MailMetadata, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var metadata MailMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, err
	}
	return &metadata, nil
}

// SaveMetadata marshals the MailMetadata struct to JSON and writes it to filePath.
// It creates the file if it doesn't exist, or truncates it if it does.
func SaveMetadata(filePath string, metadata *MailMetadata) error {
	data, err := json.MarshalIndent(metadata, "", "  ") // Use MarshalIndent for readability
	if err != nil {
		return err
	}

	// WriteFile creates the file if it doesn't exist, and truncates it otherwise.
	// Permissions 0640: owner can read/write, group can read, others no access.
	return os.WriteFile(filePath, data, 0640)
}
