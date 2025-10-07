package packet

import (
	"encoding/binary"
	"fmt"
)

// Parse inspects a WireGuard packet and extracts the message type and relevant indices.
// Returns a Message struct with the parsed information, or an error if the packet is invalid.
func Parse(data []byte) (*Message, error) {
	if len(data) < 4 {
		return nil, fmt.Errorf("packet too small: %d bytes", len(data))
	}

	// First byte contains the message type, followed by 3 reserved bytes
	msgType := MessageType(data[0])

	msg := &Message{
		Type: msgType,
		Data: data,
	}

	switch msgType {
	case MessageInitiationType:
		return parseInitiation(msg, data)
	case MessageResponseType:
		return parseResponse(msg, data)
	case MessageCookieReplyType:
		return parseCookieReply(msg, data)
	case MessageTransportType:
		return parseTransport(msg, data)
	default:
		return nil, fmt.Errorf("unknown message type: %d", msgType)
	}
}

// parseInitiation extracts the sender index from a handshake initiation message
func parseInitiation(msg *Message, data []byte) (*Message, error) {
	if len(data) != MessageInitiationSize {
		return nil, fmt.Errorf("invalid initiation message size: expected %d, got %d",
			MessageInitiationSize, len(data))
	}

	// Sender index is at bytes 4-7 (uint32 little-endian)
	sender := binary.LittleEndian.Uint32(data[4:8])
	msg.Sender = &sender

	return msg, nil
}

// parseResponse extracts sender and receiver indices from a handshake response message
func parseResponse(msg *Message, data []byte) (*Message, error) {
	if len(data) != MessageResponseSize {
		return nil, fmt.Errorf("invalid response message size: expected %d, got %d",
			MessageResponseSize, len(data))
	}

	// Sender index is at bytes 4-7 (uint32 little-endian)
	sender := binary.LittleEndian.Uint32(data[4:8])
	msg.Sender = &sender

	// Receiver index is at bytes 8-11 (uint32 little-endian)
	receiver := binary.LittleEndian.Uint32(data[8:12])
	msg.Receiver = &receiver

	return msg, nil
}

// parseCookieReply extracts the receiver index from a cookie reply message
func parseCookieReply(msg *Message, data []byte) (*Message, error) {
	if len(data) != MessageCookieReplySize {
		return nil, fmt.Errorf("invalid cookie reply message size: expected %d, got %d",
			MessageCookieReplySize, len(data))
	}

	// Receiver index is at bytes 4-7 (uint32 little-endian)
	receiver := binary.LittleEndian.Uint32(data[4:8])
	msg.Receiver = &receiver

	return msg, nil
}

// parseTransport extracts the receiver index from a transport data message
func parseTransport(msg *Message, data []byte) (*Message, error) {
	if len(data) < MessageTransportHeaderSize {
		return nil, fmt.Errorf("invalid transport message size: minimum %d, got %d",
			MessageTransportHeaderSize, len(data))
	}

	// Receiver index is at bytes 4-7 (uint32 little-endian)
	receiver := binary.LittleEndian.Uint32(data[MessageTransportOffsetReceiver : MessageTransportOffsetReceiver+4])
	msg.Receiver = &receiver

	return msg, nil
}
