package relay

import (
	"encoding/binary"
	"net"
	"testing"

	"github.com/drio/spanza/packet"
)

// Helper to create a test initiation packet (type 1, sender index only)
func makeInitiationPacket(senderIndex uint32) []byte {
	data := make([]byte, packet.MessageInitiationSize)
	data[0] = packet.MessageInitiationType
	binary.LittleEndian.PutUint32(data[4:8], senderIndex)
	return data
}

// Helper to create a test response packet (type 2, sender + receiver indices)
func makeResponsePacket(senderIndex, receiverIndex uint32) []byte {
	data := make([]byte, packet.MessageResponseSize)
	data[0] = packet.MessageResponseType
	binary.LittleEndian.PutUint32(data[4:8], senderIndex)
	binary.LittleEndian.PutUint32(data[8:12], receiverIndex)
	return data
}

// Helper to create a test transport packet (type 4, receiver index only)
func makeTransportPacket(receiverIndex uint32) []byte {
	data := make([]byte, packet.MessageTransportHeaderSize+32) // header + some payload
	data[0] = packet.MessageTransportType
	binary.LittleEndian.PutUint32(data[4:8], receiverIndex)
	return data
}

func TestProcessorInitiation(t *testing.T) {
	registry := NewRegistry()
	processor := NewProcessor(registry)

	sourceAddr := &net.UDPAddr{IP: net.IPv4(10, 0, 0, 1), Port: 51820}
	source := NewUDPEndpoint(sourceAddr)

	// Process initiation packet
	initiationPacket := makeInitiationPacket(12345)
	dest, err := processor.ProcessPacket(initiationPacket, source)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Initiation has no receiver, so dest should be nil
	if dest != nil {
		t.Errorf("expected nil destination for initiation, got %v", dest)
	}

	// Sender should be registered
	registered := registry.Lookup(12345)
	if registered == nil {
		t.Fatal("expected sender to be registered")
	}
	if !registered.Equal(source) {
		t.Errorf("registered endpoint mismatch: expected %v, got %v", source, registered)
	}
}

func TestProcessorResponse(t *testing.T) {
	registry := NewRegistry()
	processor := NewProcessor(registry)

	// Pre-register the receiver (peer who sent initiation)
	receiverAddr := &net.UDPAddr{IP: net.IPv4(10, 0, 0, 1), Port: 51820}
	receiverEndpoint := NewUDPEndpoint(receiverAddr)
	registry.Register(11111, receiverEndpoint)

	// Process response from a different peer
	senderAddr := &net.UDPAddr{IP: net.IPv4(10, 0, 0, 2), Port: 51821}
	senderEndpoint := NewUDPEndpoint(senderAddr)

	responsePacket := makeResponsePacket(22222, 11111)
	dest, err := processor.ProcessPacket(responsePacket, senderEndpoint)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should return receiver's endpoint for forwarding
	if dest == nil {
		t.Fatal("expected destination endpoint, got nil")
	}
	if !dest.Equal(receiverEndpoint) {
		t.Errorf("destination mismatch: expected %v, got %v", receiverEndpoint, dest)
	}

	// Sender should now be registered
	registered := registry.Lookup(22222)
	if registered == nil {
		t.Fatal("expected sender to be registered")
	}
	if !registered.Equal(senderEndpoint) {
		t.Errorf("registered endpoint mismatch: expected %v, got %v", senderEndpoint, registered)
	}
}

func TestProcessorTransport(t *testing.T) {
	registry := NewRegistry()
	processor := NewProcessor(registry)

	// Register the receiver
	receiverAddr := &net.UDPAddr{IP: net.IPv4(10, 0, 0, 5), Port: 51825}
	receiverEndpoint := NewUDPEndpoint(receiverAddr)
	registry.Register(55555, receiverEndpoint)

	// Process transport packet from unknown sender
	senderAddr := &net.UDPAddr{IP: net.IPv4(10, 0, 0, 3), Port: 51823}
	senderEndpoint := NewUDPEndpoint(senderAddr)

	transportPacket := makeTransportPacket(55555)
	dest, err := processor.ProcessPacket(transportPacket, senderEndpoint)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should return receiver's endpoint
	if dest == nil {
		t.Fatal("expected destination endpoint, got nil")
	}
	if !dest.Equal(receiverEndpoint) {
		t.Errorf("destination mismatch: expected %v, got %v", receiverEndpoint, dest)
	}

	// Transport packets don't have sender index, so sender shouldn't be registered
	if registry.Count() != 1 {
		t.Errorf("expected 1 peer in registry, got %d", registry.Count())
	}
}

func TestProcessorUnknownReceiver(t *testing.T) {
	registry := NewRegistry()
	processor := NewProcessor(registry)

	sourceAddr := &net.UDPAddr{IP: net.IPv4(10, 0, 0, 1), Port: 51820}
	source := NewUDPEndpoint(sourceAddr)

	// Send response to unknown receiver
	responsePacket := makeResponsePacket(12345, 99999)
	dest, err := processor.ProcessPacket(responsePacket, source)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Destination should be nil (unknown receiver)
	if dest != nil {
		t.Errorf("expected nil destination for unknown receiver, got %v", dest)
	}

	// Sender should still be registered
	registered := registry.Lookup(12345)
	if registered == nil {
		t.Fatal("expected sender to be registered")
	}
}

func TestProcessorInvalidPacket(t *testing.T) {
	registry := NewRegistry()
	processor := NewProcessor(registry)

	sourceAddr := &net.UDPAddr{IP: net.IPv4(10, 0, 0, 1), Port: 51820}
	source := NewUDPEndpoint(sourceAddr)

	// Invalid packet (too small)
	invalidPacket := []byte{1, 2}
	_, err := processor.ProcessPacket(invalidPacket, source)

	if err == nil {
		t.Fatal("expected error for invalid packet, got nil")
	}
}

func TestProcessorEndpointUpdate(t *testing.T) {
	registry := NewRegistry()
	processor := NewProcessor(registry)

	// First location for peer
	addr1 := &net.UDPAddr{IP: net.IPv4(10, 0, 0, 1), Port: 51820}
	endpoint1 := NewUDPEndpoint(addr1)

	initiationPacket := makeInitiationPacket(12345)
	_, _ = processor.ProcessPacket(initiationPacket, endpoint1)

	// Peer roams to new location
	addr2 := &net.UDPAddr{IP: net.IPv4(10, 0, 0, 2), Port: 51821}
	endpoint2 := NewUDPEndpoint(addr2)

	_, _ = processor.ProcessPacket(initiationPacket, endpoint2)

	// Should have updated to new endpoint
	registered := registry.Lookup(12345)
	if !registered.Equal(endpoint2) {
		t.Errorf("expected updated endpoint %v, got %v", endpoint2, registered)
	}
}
