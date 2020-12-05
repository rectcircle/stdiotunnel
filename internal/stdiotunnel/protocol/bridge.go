package protocol

import (
	"errors"
	"fmt"
	"io"
	"net"

	"github.com/rectcircle/stdiotunnel/internal/variable"
	"github.com/rectcircle/stdiotunnel/tools"
)

// Bridge - Bridge, manage all tunnels
type Bridge struct {
	ReadChannel <-chan Segment
	ReadClosed  <-chan error
	WriteChanel chan<- Segment
	WriteClosed <-chan error
	IsClient    bool
	Tunnels     []Tunnel
}

// NewBridge - Create a Bridge to serve
func NewBridge(reader io.ReadCloser, writer io.WriteCloser, IsClient bool) (bridge *Bridge) {
	readChannel, readClosed := DeserializeFromReader(reader)
	writeChanel, writeClosed := SerializeToWriter(writer)
	bridge = &Bridge{
		ReadChannel: readChannel,
		ReadClosed:  readClosed,
		WriteChanel: writeChanel,
		WriteClosed: writeClosed,
		IsClient:    IsClient,
		Tunnels:     make([]Tunnel, 1, variable.MaxVirtualConnection+1), // connectID = 0 not use
	}
	return
}

// ClientNewTunnel - new a Tunnel from client
func (bridge *Bridge) ClientNewTunnel(conn io.ReadWriteCloser) (VID uint16, Closed <-chan error) {
	VID = uint16(len(bridge.Tunnels))
	c := make(chan error, 1)
	tunnel := Tunnel{
		bridge.WriteChanel,
		conn,
		VID,
		c,
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
		bridge.WriteChanel <- NewRequestSegment(tunnel.VID)
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
				bridge.WriteChanel,
				conn,
				VID,
				make(chan error, 1),
			}
			if tunnel == nil {
				bridge.Tunnels = append(bridge.Tunnels, t)
			} else {
				bridge.Tunnels[VID] = t
			}
			tunnel = &bridge.Tunnels[VID]
			// response `MethodCloseConn`
			if err != nil /*&& !bridge.IsClient*/ { // must is Server
				tunnel.StartClose(bridge.IsClient, err)
				continue
			}
			// start forward
			go tunnel.Forward(bridge.IsClient)
			// response `MethodAckConn`
			bridge.WriteChanel <- NewAckSegment(VID)
		case MethodAckConn: // client handle `MethodAckConn`
			go tunnel.Forward(bridge.IsClient)
		case MethodSendData: // client or server handle `MethodSendData`
			if tunnel.Conn != nil {
				_, err := tunnel.Conn.Write(segment.Payload)
				if err != nil {
					tunnel.StartClose(bridge.IsClient, err)
				}
			}
		case MethodCloseConn: // client or server handle `MethodCloseConn`
			var err error = nil
			if segment.PayloadLength != 0 {
				err = errors.New(string(segment.Payload))
			}
			tunnel.HandleCloseConnSegment(bridge.IsClient, err)
		case MethodHeartbeat:
			// Nothing
		}
	}
}

// Tunnel - handle virtual connection
type Tunnel struct {
	WriteChanel chan<- Segment
	Conn        io.ReadWriteCloser
	VID         uint16
	Closed      chan<- error
}

// Forward - Client/Server Read from conn and send to writeChanel
func (tunnel *Tunnel) Forward(IsClient bool) {
	buffer := make([]byte, 4096)
	for tunnel.Conn != nil {
		n, err := tunnel.Conn.Read(buffer)
		if err != nil {
			if tunnel.Conn != nil {
				tunnel.StartClose(IsClient, err)
			}
			break
		}
		tunnel.WriteChanel <- NewSendDataSegment(tunnel.VID, buffer[:n])
	}
}

// StartClose - Start close a virtual connection
func (tunnel *Tunnel) StartClose(IsClient bool, err error) {
	VID := tunnel.VID
	if !IsClient {
		// Server
		tunnel.Close(IsClient, err)
	}
	tunnel.NoticeRemoteClose(VID, err)
}

// HandleCloseConnSegment - handle MethodCloseConn segment
func (tunnel *Tunnel) HandleCloseConnSegment(IsClient bool, err error) {
	VID := tunnel.VID
	if IsClient {
		// Client
		tunnel.Close(IsClient, err)
	} else {
		// Server
		tunnel.Close(IsClient, err)
		tunnel.NoticeRemoteClose(VID, err)
	}
}

// NoticeRemoteClose - Notice remote to close
func (tunnel *Tunnel) NoticeRemoteClose(VID uint16, err error) {
	tunnel.WriteChanel <- NewCloseSegment(VID, err)
}

// Close - close and reset tunnel
func (tunnel *Tunnel) Close(IsClient bool, err error) {
	tools.TraceF("%s half has closed: VID = %d\n",
		tools.If(IsClient, "Client", "Server"),
		tunnel.VID)
	if tunnel.VID != 0 {
		tunnel.Closed <- err
		close(tunnel.Closed)
		tunnel.VID = 0
	}
	if tunnel.Conn != nil {
		tunnel.Conn.Close()
		tunnel.Conn = nil
	}
}
