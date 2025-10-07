package packet

// Message type constants
const (
	MessageInitiationType  = 1
	MessageResponseType    = 2
	MessageCookieReplyType = 3
	MessageTransportType   = 4
)

// Message size constants
const (
	MessageInitiationSize      = 148
	MessageResponseSize        = 92
	MessageCookieReplySize     = 64
	MessageTransportHeaderSize = 16
)

// Field offset constants for transport messages
const (
	MessageTransportOffsetReceiver = 4
	MessageTransportOffsetCounter  = 8
	MessageTransportOffsetContent  = 16
)

// MessageType represents the type of WireGuard message
type MessageType uint8

// String returns the string representation of the message type
func (mt MessageType) String() string {
	switch mt {
	case MessageInitiationType:
		return "Initiation"
	case MessageResponseType:
		return "Response"
	case MessageCookieReplyType:
		return "CookieReply"
	case MessageTransportType:
		return "Transport"
	default:
		return "Unknown"
	}
}

// Message represents a parsed WireGuard message with extracted indices
type Message struct {
	Type     MessageType
	Sender   *uint32 // Index of sender (present in initiation, response, and transport)
	Receiver *uint32 // Index of receiver (present in response and transport)
	Data     []byte  // Raw message data
}
