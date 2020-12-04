package protocol

import (
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
	Tunnels     []Tunnel
}

// NewBridge - Create a Bridge to serve
func NewBridge(reader io.ReadCloser, writer io.WriteCloser) (bridge *Bridge) {
	readChannel, readClosed := DeserializeFromReader(reader)
	writeChanel, writeClosed := SerializeToWriter(writer)
	bridge = &Bridge{
		ReadChannel: readChannel,
		ReadClosed:  readClosed,
		WriteChanel: writeChanel,
		WriteClosed: writeClosed,
		Tunnels:     make([]Tunnel, 1, variable.MaxVirtualConnection+1), // connectID = 0 not use
	}
	return
}

// ClientNewTunnel - new a Tunnel from client
func (bridge *Bridge) ClientNewTunnel(conn io.ReadWriteCloser) error {
	VID := uint16(len(bridge.Tunnels))
	tunnel := Tunnel{
		bridge.WriteChanel,
		conn,
		0,
	}
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
	if tunnel.VID <= variable.MaxVirtualConnection {
		bridge.WriteChanel <- NewRequestSegment(tunnel.VID)
		return nil
	}
	// error
	return fmt.Errorf("Connection exhausted (%d)", variable.MaxVirtualConnection)
}

// ClientServe - Client receive from readChannel and do something
func (bridge *Bridge) ClientServe() {
	bridge.Serve("", 0)
}

// ServerServe - Server receive from readChannel and do something
func (bridge *Bridge) ServerServe(host string, port uint16) {
	bridge.Serve(host, port)
}

// Serve - receive from readChannel and do something
func (bridge *Bridge) Serve(host string, port uint16) {
	for segment := range bridge.ReadChannel {
		var (
			tunnel *Tunnel = nil
			VID            = segment.VID
		)
		// get the tunnel
		if VID < uint16(len(bridge.Tunnels)) {
			tunnel = &bridge.Tunnels[VID]
		}
		switch segment.Method {
		case MethodReqConn: // server handle `MethodReqConn`
			addr := tools.ToAddressString(host, port)
			conn, err := net.Dial("tcp", addr)
			// response `MethodCloseConn`
			if err != nil {
				bridge.Tunnels[VID].SendCloseSegment(err)
				conn.Close()
				continue
			}
			// register a tunnel
			t := Tunnel{
				bridge.WriteChanel,
				conn,
				VID,
			}
			if tunnel == nil {
				bridge.Tunnels = append(bridge.Tunnels, t)
			} else {
				bridge.Tunnels[VID] = t
			}
			tunnel = &bridge.Tunnels[VID]
			// start forward
			go tunnel.Forward()
			// response `MethodAckConn`
			bridge.WriteChanel <- NewAckSegment(VID)
		case MethodAckConn: // client handle `MethodAckConn`
			go tunnel.Forward()
		case MethodSendData: // client or server handle `MethodSendData`
			_, err := tunnel.Conn.Write(segment.Payload)
			if err != nil {
				tunnel.SendCloseSegment(err)
				tunnel.Close()
				continue
			}
		case MethodCloseConn: // client or server handle `MethodCloseConn`
			tunnel.Close()
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
}

// Forward - Client/Server Read from conn and send to writeChanel
func (tunnel *Tunnel) Forward() {
	buffer := make([]byte, 4096)
	for {
		n, err := tunnel.Conn.Read(buffer)
		if err != nil {
			tunnel.SendCloseSegment(err)
			tunnel.Close()
			break
		}
		tunnel.WriteChanel <- NewSendDataSegment(tunnel.VID, buffer[:n])
	}
}

// SendCloseSegment - close and reset this tunnel
func (tunnel *Tunnel) SendCloseSegment(err error) {
	tunnel.WriteChanel <- NewCloseSegment(tunnel.VID, err)
	tunnel.Conn.Close()
	tunnel.VID = 0
}

// Close - close and reset tunnel
func (tunnel *Tunnel) Close() {
	tunnel.Conn.Close()
	tunnel.VID = 0
}
