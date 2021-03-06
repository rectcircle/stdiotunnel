package stdiotunnel

import (
	"bytes"
	"errors"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"

	"github.com/creack/pty"
	"github.com/rectcircle/stdiotunnel/internal/variable"
	"github.com/rectcircle/stdiotunnel/tools"
	"golang.org/x/term"
)

// StartClient - run client
func StartClient(host string, port uint16, interactive bool, command string) {
	// Split command
	commandAndArgs := strings.Fields(command)
	if len(commandAndArgs) == 0 {
		tools.LogAndExitIfErr(errors.New("The command is not allowed to be an empty string"))
	}

	cmd := exec.Command(commandAndArgs[0], commandAndArgs[1:]...)
	var (
		writer io.WriteCloser = nil
		reader io.ReadCloser  = nil
	)

	if interactive {
		// Enable interactive
		f := startCommandWithPtyAndInit(cmd)
		writer = f
		reader = f
	} else {
		// Disable interactive
		var err error = nil
		writer, err = cmd.StdinPipe()
		if err != nil {
			tools.LogAndExitIfErr(err)
		}
		reader, err = cmd.StdoutPipe()
		if err != nil {
			tools.LogAndExitIfErr(err)
		}
		err = cmd.Start()
		if err != nil {
			tools.LogAndExitIfErr(err)
		}
	}
	log.Println(reader, writer)
	// Start Tunnel Server
	addr := tools.ToAddressString(host, port)
	// Listen to tcp addr
	listener, err := net.Listen("tcp", addr)
	tools.LogAndExitIfErr(err)
	for {
		// Wait accept connection
		conn, err := listener.Accept()
		tools.LogAndExitIfErr(err)
		log.Printf("Client %s connection success\n", conn.RemoteAddr().String())
		// Serve a client connection
		// go serve(ctx, conn)
	}
}

func startCommandWithPtyAndInit(cmd *exec.Cmd) (ptyFile *os.File) {
	var err error
	ptyFile, err = pty.Start(cmd)
	if err != nil {
		tools.LogAndExitIfErr(err)
	}

	// Handle pty size.
	termWinChangeChannel := make(chan os.Signal, 1)
	signal.Notify(termWinChangeChannel, syscall.SIGWINCH)
	go func() {
		for range termWinChangeChannel {
			if err := pty.InheritSize(os.Stdin, ptyFile); err != nil {
				log.Printf("error resizing pty: %s", err)
			}
		}
	}()
	termWinChangeChannel <- syscall.SIGWINCH // Initial resize.

	// Set stdin in raw mode.
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		panic(err)
	}

	closeAndRestore := func() {
		signal.Stop(termWinChangeChannel)
		close(termWinChangeChannel)
		term.Restore(int(os.Stdin.Fd()), oldState)
	}

	// Close and restore
	defer closeAndRestore()

	initDone := make(chan bool)

	// Handle stdin
	go func(initDone <-chan bool) {
		for {
			select {
			case buffer := <-tools.StdinToChannel():
				select {
				case <-initDone:
					return
				default:
					_, err := ptyFile.Write(buffer)
					tools.LogAndExitIfErr(err)
				}
			case <-initDone:
				return
			}
		}
	}(initDone)

	// Handle stdout
	// check trigger and notice stdin handle return
	var (
		buffer           = make([]byte, 4096, 4096)
		targetTrigger    = []byte(variable.StdoutReadyTrigger)
		needCheckTrigger = make([]byte, 0, 4096)
	)
	for {
		n, err := ptyFile.Read(buffer)
		if err != nil {
			closeAndRestore()
			if err.Error() == "EOF" {
				tools.LogAndExitIfErr(errors.New("EOF: command not allow exit on init stage"))
			}
			tools.LogAndExitIfErr(err)
		}
		for _, b := range buffer[:n] {
			if len(needCheckTrigger) > 0 {
				l := len(targetTrigger) - 1
				if l > len(needCheckTrigger)-1 {
					l = len(needCheckTrigger)
				}
				needCheckTrigger = needCheckTrigger[1:l:cap(needCheckTrigger)]
			}
			needCheckTrigger = append(needCheckTrigger, b)
			if bytes.Equal(needCheckTrigger, targetTrigger) {
				close(initDone)
				return
			}
		}
		os.Stdout.Write(buffer[:n])
	}
}
