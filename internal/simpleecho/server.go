package simpleecho

import (
	"context"
	"io"
	"log"
	"net"

	"github.com/rectcircle/stdiotunnel/tools"
)

// ListenAndServe - start a echo server
// and bind to `host:port` of TCP
func ListenAndServe(host string, port uint16) {
	addr := tools.ToAddressString(host, port)
	// Listen to tcp addr
	listener, err := net.Listen("tcp", addr)
	tools.LogAndExitIfErr(err)
	log.Printf("Start a Echo Server Success! on %s\n", addr)
	// Create a context
	ctx := context.Background()
	for {
		// Wait accept connection
		conn, err2 := listener.Accept()
		tools.LogAndExitIfErr(err2)
		log.Printf("Client %s connection success\n", conn.RemoteAddr().String())
		// Serve a client connection
		go serve(ctx, conn)
	}
}

func serve(ctx context.Context, conn net.Conn) {
	// copy to conn.Write from conn.Read
	n, err := io.Copy(conn, conn)
	reason := "client close"
	if err != nil {
		reason = err.Error()
	}
	log.Printf(
		"client %s connection close, write %d byte, reason: %s\n",
		conn.RemoteAddr().String(),
		n,
		reason,
	)
	conn.Close()
}
