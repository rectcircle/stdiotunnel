package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/rectcircle/stdiotunnel/internal/simplesshd"
)

func parseArgs() (host string, port uint16) {
	var (
		portUint64 uint
		help       bool
	)
	host = "127.0.0.1"
	// Due to security, not allow config host
	// flag.StringVar(&host ,"h", "127.0.0.1", "host")
	flag.UintVar(&portUint64, "p", 20022, "port")
	flag.BoolVar(&help, "help", false, "output this help")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Start a Sample sshd Server (Public Auth)\nUsage of `%s`:\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()
	if help {
		flag.Usage()
		os.Exit(0)
	}
	if portUint64 >= (1 << 16) {
		os.Stderr.WriteString("error: port must is uint16\n")
		os.Exit(2)
	}
	port = uint16(portUint64)
	return
}

func main() {
	simplesshd.ListenAndServe(parseArgs())
}
