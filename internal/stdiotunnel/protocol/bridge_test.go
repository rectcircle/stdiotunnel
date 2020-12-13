package protocol

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"testing"
	"time"

	"github.com/rectcircle/stdiotunnel/internal/variable"
	"github.com/rectcircle/stdiotunnel/tools"
)

// simulation a echo service
// 1. As a echo service
//    W --->  -------------
//           | EchoService |
//    R <---  -------------
//
// 2. simulation Pipeline or connection
//            -------------
//   W --->  | EchoService | ---> R
//            -------------
//            -------------
//   R <---  | EchoService | <--- W
//            -------------
type EchoService struct {
	closed chan bool
	buffer chan []byte
}

func NewEchoService() *EchoService {
	return &EchoService{
		make(chan bool),
		make(chan []byte),
	}
}

func (s *EchoService) Read(p []byte) (n int, err error) {
	select {
	case b := <-s.buffer:
		l1 := len(p)
		l2 := len(b)
		if l1 < l2 {
			n = l1
			s.Write(b[n:])
		} else {
			n = l2
		}
		copy(p[:n], b[:n])
	case <-s.closed:
		n = 0
		err = errors.New("Closed")
	}
	return
}

func (s *EchoService) Write(p []byte) (n int, err error) {
	select {
	case <-s.closed:
		n = 0
		err = errors.New("Closed")
	default:
		n = len(p)
		buffer := make([]byte, n)
		copy(buffer, p)
		go func() {
			s.buffer <- buffer
		}()
	}
	return
}

func (s *EchoService) Close() error {
	log.Printf("Echo server has closed")
	select {
	case <-s.closed:
		return errors.New("Closed")
	default:
		close(s.closed)
		return nil
	}
}

func simulateCreateNetConn(host string, port uint16) (io.ReadWriteCloser, error) {
	log.Printf("Create a connection to %s", tools.ToAddressString(host, port))
	return NewEchoService(), nil
}

type SimulatedConn struct {
	RW       *EchoService
	WR       *EchoService
	IsClient bool
}

func NewSimulatedConn() (client, server *SimulatedConn) {
	RW := NewEchoService()
	WR := NewEchoService()
	client = &SimulatedConn{
		RW, WR, true,
	}
	server = &SimulatedConn{
		RW, WR, false,
	}
	return
}

func (s *SimulatedConn) Read(p []byte) (n int, err error) {
	if s.IsClient {
		return s.RW.Read(p)
	}
	return s.WR.Read(p)
}

func (s *SimulatedConn) Write(p []byte) (n int, err error) {
	if s.IsClient {
		return s.WR.Write(p)
	}
	return s.RW.Write(p)
}

func (s *SimulatedConn) Close() error {
	log.Printf("<-RW- will close")
	err1 := s.RW.Close()
	log.Printf("-WR-> will close")
	err2 := s.WR.Close()
	if err1 != nil {
		return err1
	}
	if err2 != nil {
		return err2
	}
	return nil
}

func checkEchoServiceNoClose(es io.ReadWriteCloser, t *testing.T) {
	want := []byte("test")
	n, err := es.Write(want)
	if err != nil {
		t.Errorf("EchoService.Write want err = nil, got err = %v", err)
	}
	if n != len(want) {
		t.Errorf("EchoService.Write want n = %d, got n = %d", len(want), n)
	}
	got := make([]byte, 4096)
	n, err = es.Read(got)
	if err != nil {
		t.Errorf("EchoService.Read want err = nil, got err = %v", err)
	}
	if n != len(want) {
		t.Errorf("EchoService.Read want n = %d, got n = %d", len(want), n)
	}
	if !bytes.Equal(want, got[:4]) {
		t.Errorf("EchoService.Read want = %s, got = %s", string(want), string(got))
	}
	n, err = es.Write(want)
	for i := 0; i < len(want); i++ {
		got := make([]byte, 1)
		n, err = es.Read(got)
		if got[0] != want[i] {
			t.Errorf("EchoService.Read want[i] = %s, got[i] = %s", string(want[i:i+1]), string(got))
		}
	}
}

func checkEchoService(es io.ReadWriteCloser, t *testing.T) {
	want := []byte("test")
	checkEchoServiceNoClose(es, t)
	es.Close()
	_, err := es.Write(want)
	if err == nil {
		t.Errorf("EchoService.Write should return err")
	}
}

func TestEchoService(t *testing.T) {
	t.Run("TestEchoService", func(t *testing.T) {
		// test EchoService simulation
		es := NewEchoService()
		checkEchoService(es, t)
	})
}

func exp() {
	Closed := make(chan bool, 1)
	select {
	case Closed <- true:
		fmt.Println("true")
		close(Closed)
	default:
		fmt.Printf("default")
	}
	select {
	case Closed <- true:
		fmt.Println("true")
		close(Closed)
	default:
		fmt.Printf("default")
	}
}

func bridgeServeSmoke(t *testing.T) {
	pipeForClient, pipeForServer := NewSimulatedConn()
	client := NewBridge(pipeForClient, true)
	server := NewBridge(pipeForServer, false)
	// start a Serve
	go func() {
		server.Serve("localhost", 10007, simulateCreateNetConn)
	}()
	// start a client
	go func() {
		client.ClientServe()
	}()
	// create client Conn
	clientConnForClient, clientConnForServer := NewSimulatedConn()
	VID, Closed := client.ClientNewTunnel(clientConnForServer)
	log.Printf("Create a new Virtual Connection VID = %d", VID)
	checkEchoService(clientConnForClient, t)
	err := <-Closed
	log.Printf("Close a Virtual Connection VID = %d, err = %v", VID, err)
}

func bridgeServeBoundaryConnetionExhausted(t *testing.T) {
	MaxVirtualConnection := variable.MaxVirtualConnection
	variable.MaxVirtualConnection = 1
	pipeForClient, pipeForServer := NewSimulatedConn()
	client := NewBridge(pipeForClient, true)
	server := NewBridge(pipeForServer, false)
	// start a Serve
	go func() {
		server.Serve("localhost", 10007, simulateCreateNetConn)
	}()
	// start a client
	go func() {
		client.ClientServe()
	}()
	// create client Conn 1
	clientConnForClient, clientConnForServer := NewSimulatedConn()
	VID, Closed := client.ClientNewTunnel(clientConnForServer)
	log.Printf("Create a new Virtual Connection VID = %d", VID)
	// create client Conn 2
	_, clientConnForServer2 := NewSimulatedConn()
	VID2, Closed2 := client.ClientNewTunnel(clientConnForServer2)
	log.Printf("Create a new Virtual Connection VID = %d", VID2)
	err2 := <-Closed2
	log.Printf("Close a Virtual Connection VID = %d, err = %v", VID2, err2)
	// test and close conn 1
	checkEchoService(clientConnForClient, t)
	err := <-Closed
	log.Printf("Close a Virtual Connection VID = %d, err = %v", VID, err)
	// create client Conn 3
	clientConnForClient3, clientConnForServer3 := NewSimulatedConn()
	VID3, Closed3 := client.ClientNewTunnel(clientConnForServer3)
	log.Printf("Create a new Virtual Connection VID = %d", VID3)
	checkEchoService(clientConnForClient3, t)
	err3 := <-Closed3
	log.Printf("Close a Virtual Connection VID = %d, err = %v", VID3, err3)
	variable.MaxVirtualConnection = MaxVirtualConnection
}

func bridgeServeBoundaryServerStartConnError(t *testing.T) {
	MaxVirtualConnection := variable.MaxVirtualConnection
	variable.MaxVirtualConnection = 3
	pipeForClient, pipeForServer := NewSimulatedConn()
	client := NewBridge(pipeForClient, true)
	server := NewBridge(pipeForServer, false)
	// start a Serve
	go func() {
		server.Serve("localhost", 10007, func(host string, port uint16) (io.ReadWriteCloser, error) {
			return nil, fmt.Errorf("Connection to %s error", tools.ToAddressString(host, port))
		})
	}()
	// start a client
	go func() {
		client.ClientServe()
	}()
	// create client Conn
	for i := 0; i < 10; i++ {
		clientConnForClient, clientConnForServer := NewSimulatedConn()
		VID, Closed := client.ClientNewTunnel(clientConnForServer)
		log.Printf("Create a new Virtual Connection VID = %d", VID)
		// checkEchoService(clientConnForClient, t)
		err := <-Closed
		log.Printf("Close a Virtual Connection VID = %d, err = %v", VID, err)
		n, err := clientConnForClient.Write([]byte("test"))
		if n != 0 || err == nil {
			t.Errorf("EchoService.Write should return 0, Closed, but return %d, %s", n, err)
		}
		n, err = clientConnForClient.Read([]byte("test"))
		if n != 0 || err == nil {
			t.Errorf("EchoService.Read should return 0, Closed, but return %d, %s", n, err)
		}
	}
	variable.MaxVirtualConnection = MaxVirtualConnection
}

func bridgeServeBoundaryServerClose(t *testing.T) {
	MaxVirtualConnection := variable.MaxVirtualConnection
	variable.MaxVirtualConnection = 3
	pipeForClient, pipeForServer := NewSimulatedConn()
	client := NewBridge(pipeForClient, true)
	server := NewBridge(pipeForServer, false)
	var serverConn io.ReadWriteCloser = nil
	createNetConn := func(host string, port uint16) (io.ReadWriteCloser, error) {
		if serverConn != nil {
			serverConn.Close()
		}
		rw, err := simulateCreateNetConn(host, port)
		serverConn = rw
		return rw, err
	}
	// start a Serve
	go func() {
		server.Serve("localhost", 10007, createNetConn)
	}()
	// start a client
	go func() {
		client.ClientServe()
	}()
	var (
		Closed <-chan error
	)
	for i := 0; i < 10; i++ {
		// create client Conn
		clientConnForClient, clientConnForServer := NewSimulatedConn()
		VID, ClosedTmp := client.ClientNewTunnel(clientConnForServer)
		if Closed != nil {
			err := <-Closed
			log.Printf("Close a Virtual Connection VID = %d, err = %v", VID, err)
		}
		Closed = ClosedTmp
		log.Printf("Create a new Virtual Connection VID = %d", VID)
		checkEchoServiceNoClose(clientConnForClient, t)
	}
	variable.MaxVirtualConnection = MaxVirtualConnection
}

func bridgeLineBreak(t *testing.T) {
	pipeForClient, pipeForServer := NewSimulatedConn()
	client := NewBridge(pipeForClient, true)
	server := NewBridge(pipeForServer, false)
	// start a Serve
	go func() {
		server.Serve("localhost", 10007, simulateCreateNetConn)
	}()
	// start a client
	go func() {
		client.ClientServe()
	}()
	// create client Conn
	clientConnForClient, clientConnForServer := NewSimulatedConn()
	VID, Closed := client.ClientNewTunnel(clientConnForServer)
	log.Printf("Create a new Virtual Connection VID = %d", VID)
	clientConnForClient.Write([]byte("test"))
	buffer := make([]byte, 4096)
	n, err := clientConnForClient.Read(buffer)
	log.Printf("buffer := %s, err = %v", buffer[:n], err)
	pipeForServer.Close()
	clientConnForClient.Write([]byte("test"))
	err = <-Closed
	log.Printf("Close a Virtual Connection VID = %d, err = %v", VID, err)
	time.Sleep(10 * time.Millisecond)
}

func TestBridge_Serve(t *testing.T) {
	// exp()
	EnableTraceLog := variable.EnableTraceLog
	variable.EnableTraceLog = true
	t.Run("smoke", bridgeServeSmoke)
	t.Run("boundary connetion exhausted", bridgeServeBoundaryConnetionExhausted)
	t.Run("boundary server start connection error", bridgeServeBoundaryServerStartConnError)
	t.Run("boundary server close", bridgeServeBoundaryServerClose)
	t.Run("boundary line break", bridgeLineBreak)
	variable.EnableTraceLog = EnableTraceLog
}
