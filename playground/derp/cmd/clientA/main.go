package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/drio/spanza/playground/derp/client"
	"tailscale.com/derp"
)

func main() {
	serverURL := flag.String("server", "http://localhost:3340", "DERP server URL")
	peerKeyStr := flag.String("peer", "", "Peer's public key to send to")
	myKeyStr := flag.String("key", "", "Our private key")
	flag.Parse()

	if *peerKeyStr == "" {
		log.Fatal("Usage: clientA --key <our-private-key> --peer <peer-public-key>")
	}

	if *myKeyStr == "" {
		log.Fatal("Error: --key is required\nRun 'make gen-key' to generate a key, then 'make run'")
	}

	// Parse peer's public key
	peerKey, err := client.ParsePublicKey(*peerKeyStr)
	if err != nil {
		log.Fatalf("Invalid peer key: %v", err)
	}

	// Parse our private key
	privateKey, err := client.ParsePrivateKey(*myKeyStr)
	if err != nil {
		log.Fatalf("Invalid private key: %v", err)
	}

	log.Printf("=== Client A (Sender) ===")
	log.Printf("Our public key: %s", client.GetPublicKey(privateKey).String())
	log.Printf("Connecting to DERP server: %s", *serverURL)
	log.Printf("Will send messages to peer: %s", peerKey.String())
	log.Println()

	// Create DERP client
	derpClient, err := client.NewDERPClient(privateKey, *serverURL)
	if err != nil {
		log.Fatalf("Failed to create DERP client: %v", err)
	}
	defer derpClient.Close()

	log.Println("✓ Connected to DERP server")

	// Start receiver goroutine
	go receiveMessages(derpClient)

	// Send messages periodically
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	msgCount := 0
	for range ticker.C {
		msgCount++
		message := fmt.Sprintf("Hello from Client A (message #%d)", msgCount)

		log.Printf("→ Sending: %s", message)
		if err := derpClient.Send(peerKey, []byte(message)); err != nil {
			log.Printf("Failed to send message: %v", err)
		}
	}
}

// receiveMessages continuously receives messages from the DERP server
func receiveMessages(derpClient *client.DERPClient) {
	for {
		msg, err := derpClient.Recv()
		if err != nil {
			log.Printf("Receive error: %v", err)
			os.Exit(1)
		}

		// Handle different message types
		switch m := msg.(type) {
		case derp.ReceivedPacket:
			log.Printf("← Received from %s: %s",
				m.Source.ShortString(),
				string(m.Data))
		case derp.PeerGoneMessage:
			log.Printf("! Peer %s is gone (reason: %v)",
				m.Peer.ShortString(), m.Reason)
		case derp.PingMessage:
			log.Printf("← Received ping")
		case derp.PongMessage:
			log.Printf("← Received pong")
		default:
			log.Printf("← Received unknown message type: %T", msg)
		}
	}
}

