package protocol

import (
	"errors"
	"fmt"
	"io"
	"net"
	"sync"

	"github.com/rectcircle/stdiotunnel/internal/variable"
	"github.com/rectcircle/stdiotunnel/tools"
)

// Bridge - Bridge, manage all tunnels
type Bridge struct {
	reader           io.ReadCloser
	writer           io.WriteCloser
	ReadChannel      <-chan Segment
	ReadClosed       <-chan error
	WriteChannel     chan<- Segment
	WriteClosed      <-chan error
	WriteClosedError error
	WriteMutex       *sync.Mutex
	IsClient         bool
	Tunnels          []Tunnel
}

// WritableSegmentChannel - writable segment channel
type WritableSegmentChannel interface {
	Write(segment Segment) error
}

// NewBridge - Create a Bridge to serve
func NewBridge(reader io.ReadCloser, writer io.WriteCloser, IsClient bool) (bridge *Bridge) {
	readChannel, readClosed := DeserializeFromReader(reader)
	WriteChannel, writeClosed, writeMutex := SerializeToWriter(writer)
	bridge = &Bridge{
		reader:           reader,
		writer:           writer,
		ReadChannel:      readChannel,
		ReadClosed:       readClosed,
		WriteChannel:     WriteChannel,
		WriteClosed:      writeClosed,
		WriteClosedError: nil,

		WriteMutex: writeMutex,
		IsClient:   IsClient,
		Tunnels:    make([]Tunnel, 1, variable.MaxVirtualConnection+1), // connectID = 0 not use
	}
	return
}

func (bridge *Bridge) Write(segment Segment) error {
	bridge.WriteMutex.Lock()
	defer bridge.WriteMutex.Unlock()
	select {
	case err := <-bridge.WriteClosed:
		if err != nil {
			bridge.WriteClosedError = err
		}
	default:
		bridge.WriteChannel <- segment
	}
	return bridge.WriteClosedError
}

// ClientNewTunnel - new a Tunnel from client
func (bridge *Bridge) ClientNewTunnel(conn io.ReadWriteCloser) (VID uint16, Closed <-chan error) {
	VID = uint16(len(bridge.Tunnels))
	c := make(chan error, 1)
	tunnel := Tunnel{
		Conn:   conn,
		VID:    VID,
		Closed: c,
		mutex:  &sync.Mutex{},
	}
	Closed = c
	// register this virtual connetion
	if VID <= variable.MaxVirtualConnection {
		bridge.Tunnels = append(bridge.Tunnels, tunnel)
	} else {
		for i, t := range bridge.Tunnels {
			if t.VID == 0 && i != 0 {
				VID = uint16(i)
				tunnel.VID = VID
				bridge.Tunnels[i] = tunnel
				break
			}
		}
	}
	// send new connection request
	if VID <= variable.MaxVirtualConnection {
		bridge.Write(NewRequestSegment(tunnel.VID))
		return
	}
	// error
	c <- fmt.Errorf("Connection exhausted (max = %d)", variable.MaxVirtualConnection)
	conn.Close()
	close(c)
	return
}

// ClientServe - Client receive from readChannel and do something
func (bridge *Bridge) ClientServe() {
	bridge.Serve("", 0, nil)
}

// ServerServe - Server receive from readChannel and do something
func (bridge *Bridge) ServerServe(host string, port uint16) {
	bridge.Serve(host, port, func(host string, port uint16) (io.ReadWriteCloser, error) {
		addr := tools.ToAddressString(host, port)
		conn, err := net.Dial("tcp", addr)
		return conn, err
	})
}

// CreateNetConn - Create TCP network connection
type CreateNetConn func(host string, port uint16) (io.ReadWriteCloser, error)

// Serve - receive from readChannel and do something
func (bridge *Bridge) Serve(host string, port uint16, createNetConn CreateNetConn) {
	for segment := range bridge.ReadChannel {
		var (
			tunnel *Tunnel = nil
			VID            = segment.VID
		)
		tools.TraceF("%s receive: VID = %d, Method = %d\n",
			tools.If(bridge.IsClient, "Client", "Server"),
			segment.VID, segment.Method)
		// get the tunnel
		if VID < uint16(len(bridge.Tunnels)) {
			tunnel = &bridge.Tunnels[VID]
		}
		switch segment.Method {
		case MethodReqConn: // server handle `MethodReqConn`
			conn, err := createNetConn(host, port)
			// register a tunnel
			t := Tunnel{
				Conn:   conn,
				VID:    VID,
				Closed: make(chan error, 1),
				mutex:  &sync.Mutex{},
			}
			if tunnel == nil {
				bridge.Tunnels = append(bridge.Tunnels, t)
			} else {
				bridge.Tunnels[VID] = t
			}
			tunnel = &bridge.Tunnels[VID]
			// response `MethodCloseConn`
			if err != nil /*&& !bridge.IsClient*/ { // must is Server
				tunnel.StartClose(bridge, bridge.IsClient, err)
				continue
			}
			// start forward
			go tunnel.Forward(bridge.reader, bridge.ReadClosed, bridge, bridge.IsClient)
			// response `MethodAckConn`
			bridge.Write(NewAckSegment(VID))
		case MethodAckConn: // client handle `MethodAckConn`
			go tunnel.Forward(bridge.reader, bridge.ReadClosed, bridge, bridge.IsClient)
		case MethodSendData: // client or server handle `MethodSendData`
			tunnel.WriteToConn(segment.Payload, bridge, bridge.IsClient)
		case MethodCloseConn: // client or server handle `MethodCloseConn`
			var err error = nil
			if segment.PayloadLength != 0 {
				err = errors.New(string(segment.Payload))
			}
			tunnel.HandleCloseConnSegment(bridge, bridge.IsClient, err)
		case MethodHeartbeat:
			// Nothing
		}
	}
	// receive reader Closed
	err := <-bridge.ReadClosed
	tools.TraceF("%s Serve receive ReadClosed: err = %v\n",
		tools.If(bridge.IsClient, "Client", "Server"),
		err)
	// close all virtual connection
	bridge.CloseTunnels()
	// close writer if writer is not closed
	select {
	case <-bridge.WriteClosed:
	default:
		bridge.writer.Close()
	}
}

// CloseTunnels - close all virtual connection
func (bridge *Bridge) CloseTunnels() {
	for _, tunnel := range bridge.Tunnels {
		if tunnel.VID != 0 {
			if tunnel.Conn != nil {
				tunnel.Close(bridge.IsClient, errors.New("line break"))
			}
		}
	}
}

// Tunnel - handle virtual connection
type Tunnel struct {
	Conn   io.ReadWriteCloser
	VID    uint16
	Closed chan<- error
	mutex  *sync.Mutex
}

// Forward - Client/Server Read from conn and send to WriteChannel
func (tunnel *Tunnel) Forward(reader io.ReadCloser, ReadCloser <-chan error, Writable WritableSegmentChannel, IsClient bool) {
	buffer := make([]byte, 4096)
	for tunnel.Conn != nil {
		// Read
		n, err := tunnel.Conn.Read(buffer)
		if err != nil {
			tools.TraceF("%s Forward has exit: VID = %d err = %v\n",
				tools.If(IsClient, "Client", "Server"),
				tunnel.VID, err)
			tunnel.mutex.Lock()
			if tunnel.Conn != nil {
				tunnel.mutex.Unlock()
				tunnel.StartClose(Writable, IsClient, err)
			} else {
				tunnel.mutex.Unlock()
			}
			break
		}
		Writable.Write(NewSendDataSegment(tunnel.VID, buffer[:n]))
	}
}

// WriteToConn - write segment.Payload to conn
func (tunnel *Tunnel) WriteToConn(buffer []byte, Writable WritableSegmentChannel, IsClient bool) (n int, err error) {
	tunnel.mutex.Lock()
	if tunnel.Conn != nil {
		n, err = tunnel.Conn.Write(buffer)
		tunnel.mutex.Unlock()
		if err != nil {
			tunnel.StartClose(Writable, IsClient, err)
		}
	} else {
		tunnel.mutex.Unlock()
		err = errors.New("conn has closed")
	}
	return
}

// StartClose - Start close a virtual connection
func (tunnel *Tunnel) StartClose(Writable WritableSegmentChannel, IsClient bool, err error) {
	VID := tunnel.VID
	if !IsClient {
		// Server
		tunnel.Close(IsClient, err)
	}
	tunnel.NoticeRemoteClose(Writable, VID, err)
}

// HandleCloseConnSegment - handle MethodCloseConn segment
func (tunnel *Tunnel) HandleCloseConnSegment(Writable WritableSegmentChannel, IsClient bool, err error) {
	VID := tunnel.VID
	if IsClient {
		// Client
		tunnel.Close(IsClient, err)
	} else {
		// Server
		tunnel.Close(IsClient, err)
		tunnel.NoticeRemoteClose(Writable, VID, err)
	}
}

// NoticeRemoteClose - Notice remote to close
func (tunnel *Tunnel) NoticeRemoteClose(Writable WritableSegmentChannel, VID uint16, err error) {
	Writable.Write(NewCloseSegment(VID, err))
}

// Close - close and reset tunnel
func (tunnel *Tunnel) Close(IsClient bool, err error) {
	tools.TraceF("%s half has closed: VID = %d\n",
		tools.If(IsClient, "Client", "Server"),
		tunnel.VID)
	tunnel.mutex.Lock()
	defer tunnel.mutex.Unlock()
	if tunnel.VID != 0 {
		tunnel.VID = 0
		tunnel.Closed <- err
		close(tunnel.Closed)
	}
	if tunnel.Conn != nil {
		Conn := tunnel.Conn
		tunnel.Conn = nil
		Conn.Close()
	}
}
