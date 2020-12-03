package stdiotunnel

import (
	"bytes"
	"encoding/binary"
	"io"
	"math"
	"net"
)

const (
	// ProtocolVersion1 - Protocol Version 1
	ProtocolVersion1 = byte(1)
)

const (
	// MethodReqConn - request virtual connection
	MethodReqConn = byte(iota + 1)
	// MethodAckConn - Ack virtual connection
	MethodAckConn
	// MethodSendData - Send data
	MethodSendData
	// MethodCloseConn - Close virtual connection
	MethodCloseConn
	// MethodHeartbeat - Heartbeat
	MethodHeartbeat
)

// A kind of stdio multiplexing private protocol implementation

// Segment - this is data Segment on stdio, use Big-Endian
type Segment struct {
	// protocol version
	Version byte
	// method
	Method byte
	// Virtual Connection ID
	VID uint16
	// payload length
	PayloadLength uint32
	// payload
	Payload []byte
}

// NewRequestSegment - new a Segment with method = MethodReqConn
func NewRequestSegment() Segment {
	return Segment{
		Version: ProtocolVersion1,
		Method:  MethodReqConn,
	}
}

// NewAckSegment - new a Segment with method = MethodAckConn
func NewAckSegment(VID uint16, ServerVID uint16) Segment {
	payload := make([]byte, 2)
	binary.BigEndian.PutUint16(payload, ServerVID)
	return Segment{
		Version:       ProtocolVersion1,
		Method:        MethodAckConn,
		VID:           VID,
		PayloadLength: 2,
		Payload:       payload,
	}
}

// NewSendDataSegment - new a Segment with method = MethodSendData
func NewSendDataSegment(VID uint16, payload []byte) Segment {
	return Segment{
		Version:       ProtocolVersion1,
		Method:        MethodSendData,
		VID:           VID,
		PayloadLength: uint32(len(payload)),
		Payload:       payload,
	}
}

// NewCloseSegment - new a Segment with method = MethodCloseConn
func NewCloseSegment(VID uint16) Segment {
	return Segment{
		Version: ProtocolVersion1,
		Method:  MethodCloseConn,
		VID:     VID,
	}
}

// NewHeartbeatSegment - new a Segment with method = MethodHeartbeat
func NewHeartbeatSegment() Segment {
	return Segment{
		Version: ProtocolVersion1,
		Method:  MethodHeartbeat,
	}
}

// Equal - Equal
func (s *Segment) Equal(other *Segment) bool {
	return s.Version == other.Version &&
		s.Method == other.Method &&
		s.VID == other.VID &&
		s.PayloadLength == other.PayloadLength &&
		bytes.Equal(s.Payload, other.Payload)
}

// Copy - deep copy the object
func (s Segment) Copy() Segment {
	payload := make([]byte, s.PayloadLength)
	copy(payload, s.Payload)
	s.Payload = payload
	return s
}

// Serialize - Serialize Segment to []byte
func (s Segment) Serialize() []byte {
	data := make([]byte, 8+s.PayloadLength)
	data[0] = s.Version
	data[1] = s.Method
	binary.BigEndian.PutUint16(data[2:4], s.VID)
	binary.BigEndian.PutUint32(data[4:8], s.PayloadLength)
	copy(data[8:], s.Payload)
	return data
}

// SerializeToWriter - start a goroutine to receive Segment from channel and write to `writer`
// if write() error, `closed` will receive a error and close the `closed` channel
func SerializeToWriter(writer io.Writer) (chan<- Segment, <-chan error) {
	segmentChannel := make(chan Segment)
	closed := make(chan error)

	go func() {
		for {
			s := <-segmentChannel
			_, err := writer.Write(s.Serialize())
			if err != nil {
				closed <- err
				close(closed)
				break
			}
		}
	}()

	return segmentChannel, closed
}

// DeserializeFromReader - start a goroutine to read and Deserialize `reader` and send to `segment`
// if read() error, `closed` will receive a error and close the `closed` channel
func DeserializeFromReader(reader io.Reader) (<-chan Segment, <-chan error) {
	segmentChannel := make(chan Segment)
	closed := make(chan error)
	go func() {
		buffer := make([]byte, 4096)
		var cache Segment
		var state segmentState
		for {
			n, err := reader.Read(buffer)
			if err != nil {
				closed <- err
				close(closed)
				return
			}
			for _, segment := range handleBytes(&cache, &state, buffer[:n]) {
				segmentChannel <- segment
			}
		}
	}()
	return segmentChannel, closed
}

type segmentState struct {
	step          int32
	cache         []byte
	dataRemaining uint32
}

const (
	segmentStateStepVersion = int32(iota)
	segmentStateStepMethod
	segmentStateStepVID
	segmentStateStepPayloadLength
	segmentStateStepPayload
)

func handleBytes(cache *Segment, state *segmentState, buffer []byte) []Segment {
	n := uint32(len(buffer))
	result := []Segment{}
	for i := uint32(0); i < n; {
		b := buffer[i]
		switch state.step {
		case segmentStateStepVersion:
			cache.Version = b
			state.step++
			i++
		case segmentStateStepMethod:
			cache.Method = b
			state.step++
			i++
		case segmentStateStepVID:
			if state.cache == nil {
				state.cache = make([]byte, 2)
				state.cache[0] = b
			} else {
				state.cache[1] = b
				cache.VID = binary.BigEndian.Uint16(state.cache)
				state.cache = nil
				state.step++
			}
			i++
		case segmentStateStepPayloadLength:
			if state.cache == nil {
				state.cache = make([]byte, 0, 4)
				state.cache = append(state.cache, b)
			} else {
				state.cache = append(state.cache, b)
				if len(state.cache) == 4 {
					cache.PayloadLength = binary.BigEndian.Uint32(state.cache)
					cache.Payload = []byte{}
					state.cache = nil
					state.dataRemaining = cache.PayloadLength
					state.step++
				}
			}
			i++
		case segmentStateStepPayload:
			lastLen := i + state.dataRemaining
			if lastLen > n {
				lastLen = n
			}
			cache.Payload = append(cache.Payload, buffer[i:lastLen]...)
			state.dataRemaining -= lastLen - i
			i = lastLen
		}
		if segmentStateStepPayload == state.step && 0 == state.dataRemaining {
			result = append(result, *cache)
			*cache = Segment{}
			*state = segmentState{}
		}
	}
	return result
}

// Bridge - Bridge, manage all tunnels
type Bridge struct {
	ReadChannel <-chan Segment
	ReadClosed  <-chan error
	WriteChanel chan<- Segment
	WriteClosed <-chan error
	Tunnels     []Tunnel
}

// NewBridge - Create a Bridge to serve
func NewBridge(reader io.ReadCloser, writer io.WriteCloser) Bridge {
	readChannel, readClosed := DeserializeFromReader(reader)
	writeChanel, writeClosed := SerializeToWriter(writer)
	return Bridge{
		ReadChannel: readChannel,
		ReadClosed:  readClosed,
		WriteChanel: writeChanel,
		WriteClosed: writeClosed,
		Tunnels:     make([]Tunnel, 0, math.MaxUint16),
	}
}

// ClientNewTunnel - new a Tunnel from client
func (*Bridge) ClientNewTunnel(conn net.Conn) {

}

// ClientServe - Client receive from readChannel and do something
func (*Bridge) ClientServe() {
}

// ServerServe - Server receive from readChannel and do something
func (*Bridge) ServerServe() {

}

// Tunnel - handle virtual connection
type Tunnel struct {
	readChannel <-chan Segment
	writeChanel chan<- Segment
	conn        net.Conn
	VID         uint16
}

// Forward - Client/Server Read from conn and send to writeChanel
func (*Tunnel) Forward() {

}
