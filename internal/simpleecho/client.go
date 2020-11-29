package simpleecho

import (
	"fmt"
	"log"
	"net"
	"os"

	"github.com/rectcircle/stdiotunnel/tools"
)

// Client - connect to echo server
func Client(host string, port uint16) {
	addr := tools.ToAddressString(host, port)
	// connect to server
	conn, err := net.Dial("tcp", addr)
	tools.LogAndExitIfErr(err)
	defer conn.Close()
	var buffer = make([]byte, 4096)
	for {
		var line string
		// read from stdin
		fmt.Scanln(&line)
		var lineLen = len(line)
		// write to connection
		conn.Write([]byte(line))
		// read from connection
		for ; lineLen > 0; {
			n, err2 := conn.Read(buffer)
			if err2 != nil {
				if err2.Error() == "EOF" {
					log.Println("Server close")
					os.Exit(0)
				}
				break
			}
			lineLen -= n
			os.Stdout.Write(buffer[:n])
		}
		// append a `\n`
		os.Stdout.WriteString("\n")
	}
}

