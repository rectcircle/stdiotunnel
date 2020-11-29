package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/rectcircle/stdiotunnel/internal/simpleecho"
)

var (
	subcommandKeyServer = "server"
	subcommandKeyClient = "client"
	subcommandKeyHelp = "help"
)

func parseArgs(subcommand string, desc string, args []string) (host string, port uint16) {
	var (
		portUint64 uint
		help bool
	)
	flagset := flag.NewFlagSet(subcommand, flag.ExitOnError)
	flagset.StringVar(&host ,"h", "127.0.0.1", "host")
	flagset.UintVar(&portUint64, "p", 20007, "port")
	flagset.BoolVar(&help, "help", false, "output this subcommand help")
	flagset.Usage = func ()  {
		fmt.Fprintf(flagset.Output(), "%s\nUsage of `%s %s`:\n", desc, os.Args[0], subcommand)
		flagset.PrintDefaults()
	}
	flagset.Parse(args[1:])
	if help {
		flagset.Usage()
		os.Exit(0)
	}
	if portUint64 >= (1 << 16) {
		os.Stderr.WriteString("error: port must is uint16\n")
		os.Exit(2)
	}
	port = uint16(portUint64)
	return
}

func helpAndExit(isErr bool) {
	stdOutOrErr := os.Stdout
	if isErr {
		stdOutOrErr = os.Stderr
	}
	fmt.Fprintf(stdOutOrErr, "A simple echo server and client for test the stdiotunnel\nUsage of %s server | client\n  -help\n         output this help\n", os.Args[0])
	if isErr {
		os.Exit(2)
	}
}

func main() {
	if len(os.Args) < 2 {
		helpAndExit(true)
	}
	switch os.Args[1] {
	case subcommandKeyServer:
		simpleecho.ListenAndServe(parseArgs(os.Args[1], "Run a echo server", os.Args[1:]))
	case subcommandKeyClient:
		simpleecho.Client(parseArgs(os.Args[1], "Connect to a echo server", os.Args[1:]))
	case subcommandKeyHelp:
		helpAndExit(false)
	default:
		helpAndExit(true)
	}
}
