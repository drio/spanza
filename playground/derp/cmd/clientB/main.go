package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/drio/spanza/playground/derp/client"
	"tailscale.com/derp"
)

func main() {
	serverURL := flag.String("server", "http://localhost:3340", "DERP server URL")
	myKeyStr := flag.String("key", "", "Our private key")
	echoBack := flag.Bool("echo", true, "Echo received messages back to sender")
	flag.Parse()

	if *myKeyStr == "" {
		log.Fatal("Error: --key is required\nRun 'make gen-key' to generate a key")
	}

	// Parse our private key
	privateKey, err := client.ParsePrivateKey(*myKeyStr)
	if err != nil {
		log.Fatalf("Invalid private key: %v", err)
	}

	log.Printf("=== Client B (Receiver) ===")
	log.Printf("Our public key: %s", client.GetPublicKey(privateKey).String())
	log.Printf("Connecting to DERP server: %s", *serverURL)
	log.Printf("Echo mode: %v", *echoBack)
	log.Println()
	log.Println("Share this public key with Client A so it can send messages to us!")
	log.Println("Waiting for messages...")
	log.Println()

	// Create DERP client
	derpClient, err := client.NewDERPClient(privateKey, *serverURL)
	if err != nil {
		log.Fatalf("Failed to create DERP client: %v", err)
	}
	defer derpClient.Close()

	log.Println("✓ Connected to DERP server")
	log.Println()

	// Receive messages loop
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

			// Echo back if enabled
			if *echoBack {
				response := fmt.Sprintf("Echo: %s", string(m.Data))
				log.Printf("→ Echoing back: %s", response)
				if err := derpClient.Send(m.Source, []byte(response)); err != nil {
					log.Printf("Failed to echo back: %v", err)
				}
			}

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
