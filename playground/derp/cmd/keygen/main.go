package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/drio/spanza/playground/derp/client"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--public" {
		// Read private key from stdin and output public key
		var cfg client.KeyConfig
		if err := json.NewDecoder(os.Stdin).Decode(&cfg); err != nil {
			fmt.Fprintf(os.Stderr, "Error reading private key: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(client.GetPublicKey(cfg.PrivateKey).String())
		return
	}

	// Generate new private key
	privateKey := client.GenerateKey()
	cfg := client.KeyConfig{PrivateKey: privateKey}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling key: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(string(data))
}
