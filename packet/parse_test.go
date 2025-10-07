package packet

import (
	"encoding/binary"
	"testing"
)

func TestParseInitiation(t *testing.T) {
	// Create a mock handshake initiation message
	data := make([]byte, MessageInitiationSize)
	binary.LittleEndian.PutUint32(data[0:4], MessageInitiationType)
	binary.LittleEndian.PutUint32(data[4:8], 0x12345678) // Sender index

	msg, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if msg.Type != MessageInitiationType {
		t.Errorf("Expected type %d, got %d", MessageInitiationType, msg.Type)
	}

	if msg.Sender == nil {
		t.Fatal("Sender index is nil")
	}

	if *msg.Sender != 0x12345678 {
		t.Errorf("Expected sender 0x12345678, got 0x%x", *msg.Sender)
	}

	if msg.Receiver != nil {
		t.Error("Receiver should be nil for initiation messages")
	}
}

func TestParseResponse(t *testing.T) {
	// Create a mock handshake response message
	data := make([]byte, MessageResponseSize)
	binary.LittleEndian.PutUint32(data[0:4], MessageResponseType)
	binary.LittleEndian.PutUint32(data[4:8], 0x11111111)  // Sender index
	binary.LittleEndian.PutUint32(data[8:12], 0x22222222) // Receiver index

	msg, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if msg.Type != MessageResponseType {
		t.Errorf("Expected type %d, got %d", MessageResponseType, msg.Type)
	}

	if msg.Sender == nil {
		t.Fatal("Sender index is nil")
	}

	if *msg.Sender != 0x11111111 {
		t.Errorf("Expected sender 0x11111111, got 0x%x", *msg.Sender)
	}

	if msg.Receiver == nil {
		t.Fatal("Receiver index is nil")
	}

	if *msg.Receiver != 0x22222222 {
		t.Errorf("Expected receiver 0x22222222, got 0x%x", *msg.Receiver)
	}
}

func TestParseTransport(t *testing.T) {
	// Create a mock transport data message
	data := make([]byte, MessageTransportHeaderSize+16) // Header + some payload
	binary.LittleEndian.PutUint32(data[0:4], MessageTransportType)
	binary.LittleEndian.PutUint32(data[4:8], 0x99999999) // Receiver index

	msg, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if msg.Type != MessageTransportType {
		t.Errorf("Expected type %d, got %d", MessageTransportType, msg.Type)
	}

	if msg.Receiver == nil {
		t.Fatal("Receiver index is nil")
	}

	if *msg.Receiver != 0x99999999 {
		t.Errorf("Expected receiver 0x99999999, got 0x%x", *msg.Receiver)
	}

	if msg.Sender != nil {
		t.Error("Sender should be nil for transport messages")
	}
}

func TestParseCookieReply(t *testing.T) {
	// Create a mock cookie reply message
	data := make([]byte, MessageCookieReplySize)
	binary.LittleEndian.PutUint32(data[0:4], MessageCookieReplyType)
	binary.LittleEndian.PutUint32(data[4:8], 0xAAAAAAAA) // Receiver index

	msg, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if msg.Type != MessageCookieReplyType {
		t.Errorf("Expected type %d, got %d", MessageCookieReplyType, msg.Type)
	}

	if msg.Receiver == nil {
		t.Fatal("Receiver index is nil")
	}

	if *msg.Receiver != 0xAAAAAAAA {
		t.Errorf("Expected receiver 0xAAAAAAAA, got 0x%x", *msg.Receiver)
	}

	if msg.Sender != nil {
		t.Error("Sender should be nil for cookie reply messages")
	}
}

func TestParseInvalidSize(t *testing.T) {
	// Test with too small packet
	data := []byte{0x01, 0x00}
	_, err := Parse(data)
	if err == nil {
		t.Error("Expected error for too small packet")
	}
}

func TestParseUnknownType(t *testing.T) {
	// Test with unknown message type
	data := make([]byte, 100)
	binary.LittleEndian.PutUint32(data[0:4], 99) // Unknown type
	_, err := Parse(data)
	if err == nil {
		t.Error("Expected error for unknown message type")
	}
}
