package simplesshd

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/binary"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"os/user"
	"path"
	"sync"
	"syscall"
	"unsafe"

	"github.com/creack/pty"
	"github.com/rectcircle/stdiotunnel/internal/variable"
	"github.com/rectcircle/stdiotunnel/tools"
	"golang.org/x/crypto/ssh"
)

// ListenAndServe - start a simple and no auth ssh server
// bind to `host:port` of TCP
// reference https://gist.github.com/jpillora/b480fde82bff51a06238
func ListenAndServe(host string, port uint16) {
	addr := tools.ToAddressString(host, port)
	// Listen to tcp addr
	listener, err := net.Listen("tcp", addr)
	tools.LogAndExitIfErr(err)
	log.Printf("Start a ssh Server Success! on %s\n", addr)
	// ssh server config: not auth
	config := &ssh.ServerConfig{
		// No Auth
		NoClientAuth: true,
	}
	// Config ssh host private key
	signer, err4 := ssh.ParsePrivateKey(readOrCreatePrivateKey())
	tools.LogAndExitIfErr(err4)
	config.AddHostKey(signer)

	for {
		// Wait accept connection
		conn, err2 := listener.Accept()
		tools.LogAndExitIfErr(err2)
		// Before use, a handshake must be performed on the incoming net.Conn.
		sshConn, channel, request, err3 := ssh.NewServerConn(conn, config)
		if err3 != nil {
			log.Printf("Client %s failed to handshake: %s\n", conn.RemoteAddr().String(), err3)
			continue
		}
		log.Printf("New SSH connection from %s (%s)\n", sshConn.RemoteAddr().String(), sshConn.ClientVersion())
		// Discard all global out-of-band Requests ???
		go ssh.DiscardRequests(request)
		// Accept all channels
		go handleChannels(channel, sshConn.RemoteAddr())
	}
}

func readOrCreatePrivateKey() []byte {
	content, err := tools.ReadOrCreateFile(
		path.Join(variable.ConfigBaseDir, variable.SSHHostKeyFileName),
		func() []byte {
			privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
			tools.LogAndExitIfErr(err)
			return pem.EncodeToMemory(&pem.Block{
				Type:  "RSA PRIVATE KEY",
				Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
			})
		},
	)
	tools.LogAndExitIfErr(err)
	return content
}

func handleChannels(channel <-chan ssh.NewChannel, remoteAttr net.Addr) {
	// Service the incoming Channel channel in go routine
	for newChannel := range channel {
		go handleChannel(newChannel, remoteAttr)
	}
}

func handleChannel(newChannel ssh.NewChannel, remoteAttr net.Addr) {
	// https://tools.ietf.org/html/rfc4254
	switch t := newChannel.ChannelType(); t {
	case "direct-tcpip":
		// ssh -L localport:remotehost:remoteport sshserver -N
		// user -> localhost:localport (local host) --- ssh tunnel ---> sshserver (sshserver host) --- network ---> remotehost:remoteport (network service)
		go handleDirectTCPIP(newChannel, remoteAttr)
	case "session":
		// At this point, we have the opportunity to reject the client's
		// request for another logical connection
		go handleSSHSession(newChannel, remoteAttr)
	default:
		// "x11" and "forwarded-tcpip" not support
		newChannel.Reject(ssh.UnknownChannelType, fmt.Sprintf("unknown channel type: %s", t))
		log.Printf("Client %s connection error: not support channel type %s", remoteAttr.String(), t)
	}
}

// direct-tcpip data struct as specified in RFC4254, Section 7.2
type localForwardChannelData struct {
	DestAddr string
	DestPort uint32

	OriginAddr string
	OriginPort uint32
}

// https://tools.ietf.org/html/rfc4254#page-17
// https://github.com/gliderlabs/ssh/blob/fb34512070c56e0b7ff4158d981baf37675e4998/tcpip.go#L28
func handleDirectTCPIP(newChannel ssh.NewChannel, remoteAttr net.Addr) {
	d := localForwardChannelData{}
	if err := ssh.Unmarshal(newChannel.ExtraData(), &d); err != nil {
		newChannel.Reject(ssh.ConnectionFailed, "error parsing forward data: "+err.Error())
		return
	}
	sourceAddress := tools.ToAddressString(d.OriginAddr, uint16(d.OriginPort))
	destAddress := tools.ToAddressString(d.DestAddr, uint16(d.DestPort))

	var dialer net.Dialer
	destConnection, err2 := dialer.Dial("tcp", destAddress)
	if err2 != nil {
		newChannel.Reject(ssh.ConnectionFailed, err2.Error())
		return
	}

	connection, requests, err3 := newChannel.Accept()
	if err3 != nil {
		destConnection.Close()
		return
	}

	log.Printf("A direct-tcpip connection success, to %s from %s", destConnection, sourceAddress)

	go ssh.DiscardRequests(requests)
	go func() {
		defer connection.Close()
		defer destConnection.Close()
		io.Copy(connection, destConnection)
	}()
	go func() {
		defer connection.Close()
		defer destConnection.Close()
		io.Copy(destConnection, connection)
	}()
}

// https://tools.ietf.org/html/rfc4254#page-11
func handleSSHSession(newChannel ssh.NewChannel, remoteAttr net.Addr) {

	var (
		enablePty          = false
		ptyFile   *os.File = nil
	)

	connection, requests, err := newChannel.Accept()
	if err != nil {
		log.Printf("Could not accept channel (%s)", err)
		return
	}

	// Sessions have out-of-band requests such as "shell", "pty-req" and "env"
	for req := range requests {
		// log.Println(req.Type)
		switch req.Type {
		case "env": // Not supported
		case "exec": // Not supported
		case "shell":
			// We only accept the default shell
			// (i.e. no command in the Payload)
			if !enablePty {
				err := startNoPtyShell(connection)
				if err != nil {
					log.Printf("Creating not pty shell error: %s)", err)
				}
			}
			if len(req.Payload) == 0 {
				req.Reply(true, nil)
			}
		case "pty-req":
			enablePty = true
			var err error = nil
			ptyFile, err = startPtyShell(connection)
			if err != nil {
				return
			}
			termLen := req.Payload[3]
			w, h := parseDims(req.Payload[termLen+4:])
			SetWinsize(ptyFile.Fd(), w, h)
			// Responding true (OK) here will let the client
			// know we have a pty ready for input
			req.Reply(true, nil)
		case "window-change":
			w, h := parseDims(req.Payload)
			SetWinsize(ptyFile.Fd(), w, h)
		}
	}
	log.Printf("Session closed from %s", remoteAttr.String())
}

func newShellCommand(connection ssh.Channel) (shell *exec.Cmd, close func()) {
	// Fire up bash for this session
	shell = exec.Command(tools.GetUnixUserShell())
	// bash := exec.Command("bash")
	//  Config shell pwd to User Home dir
	u, err2 := user.Current()
	if err2 == nil {
		shell.Dir = u.HomeDir
	}
	// Prepare teardown function
	close = func() {
		connection.Close()
		_, err := shell.Process.Wait()
		if err != nil {
			log.Printf("Failed to exit bash (%s)", err)
		}
	}
	return
}

func startPtyShell(connection ssh.Channel) (*os.File, error) {
	// New shell command
	shell, close := newShellCommand(connection)

	// Allocate a terminal for this session
	log.Printf("Creating pty shell(%s)...", shell.Path)
	ptyFile, err := pty.Start(shell)
	if err != nil {
		log.Printf("Could not start pty (%s)", err)
		close()
		return nil, err
	}

	//pipe session to bash and visa-versa
	var once sync.Once
	go func() {
		io.Copy(connection, ptyFile)
		once.Do(close)
	}()
	go func() {
		io.Copy(ptyFile, connection)
		once.Do(close)
	}()
	return ptyFile, nil
}

func startNoPtyShell(connection ssh.Channel) error {
	// New shell command
	shell, close := newShellCommand(connection)

	// Create pipe
	writer, err1 := shell.StdinPipe()
	if err1 != nil {
		connection.Close()
		return err1
	}
	reader, err2 := shell.StdoutPipe()
	if err2 != nil {
		connection.Close()
		return err2
	}

	// Allocate a no pty shell for this Sessions
	log.Printf("Creating not pty shell (%s)...", shell.Path)
	shell.Start()

	//pipe session to bash and visa-versa
	var once sync.Once
	go func() {
		io.Copy(connection, reader)
		once.Do(close)
	}()
	go func() {
		io.Copy(writer, connection)
		once.Do(close)
	}()
	return nil
}

// =======================

// parseDims extracts terminal dimensions (width x height) from the provided buffer.
func parseDims(b []byte) (uint32, uint32) {
	w := binary.BigEndian.Uint32(b)
	h := binary.BigEndian.Uint32(b[4:])
	return w, h
}

// ======================

// Winsize stores the Height and Width of a terminal.
type Winsize struct {
	Height uint16
	Width  uint16
	x      uint16 // unused
	y      uint16 // unused
}

// SetWinsize sets the size of the given pty.
func SetWinsize(fd uintptr, w, h uint32) {
	ws := &Winsize{Width: uint16(w), Height: uint16(h)}
	syscall.Syscall(syscall.SYS_IOCTL, fd, uintptr(syscall.TIOCSWINSZ), uintptr(unsafe.Pointer(ws)))
}
