package client

import (
	"encoding/json"
	"fmt"
	"os"

	"tailscale.com/types/key"
)

// KeyConfig stores a private key in JSON format
type KeyConfig struct {
	PrivateKey key.NodePrivate `json:"privateKey"`
}

// GenerateKey creates a new private key and returns it
func GenerateKey() key.NodePrivate {
	return key.NewNode()
}

// SaveKey saves a private key to a JSON file
func SaveKey(filename string, privateKey key.NodePrivate) error {
	cfg := KeyConfig{PrivateKey: privateKey}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal key: %w", err)
	}

	if err := os.WriteFile(filename, data, 0600); err != nil {
		return fmt.Errorf("write key file: %w", err)
	}

	return nil
}

// LoadKey loads a private key from a JSON file
func LoadKey(filename string) (key.NodePrivate, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return key.NodePrivate{}, fmt.Errorf("read key file: %w", err)
	}

	var cfg KeyConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return key.NodePrivate{}, fmt.Errorf("parse key file: %w", err)
	}

	return cfg.PrivateKey, nil
}

// GetPublicKey extracts the public key from a private key
func GetPublicKey(privateKey key.NodePrivate) key.NodePublic {
	return privateKey.Public()
}
