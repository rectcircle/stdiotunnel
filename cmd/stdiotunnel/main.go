package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/rectcircle/stdiotunnel/internal/stdiotunnel"
	"github.com/rectcircle/stdiotunnel/tools"
)

var (
	subcommandKeyServer = "server"
	subcommandKeyClient = "client"
	subcommandKeyHelp = "help"
)

func parseClientArgs(args []string) (host string, port uint16, interactive bool, command string) {
	var (
		portUint64 uint
		help bool
	)
	subcommand := subcommandKeyClient
	flagset := flag.NewFlagSet(subcommand, flag.ExitOnError)
	host = "127.0.0.1"
	// Due to security, not allow config host
	// flagset.StringVar(&host ,"h", "127.0.0.1", "host - bind host")
	flagset.UintVar(&portUint64, "p", 20096, "port - bind port")
	flagset.BoolVar(&interactive, "i", true, "interactive - whether start command with interactive mode (with pty mode) to initialize")
	flagset.StringVar(&command ,"c", tools.GetUnixUserShell(), "command - command to be launched")
	flagset.BoolVar(&help, "help", false, "output this subcommand help")
	flagset.Usage = func ()  {
		fmt.Fprintf(flagset.Output(), "Start a Stdio Tunnel Client\nUsage of `%s %s`:\n", os.Args[0], subcommand)
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
	fmt.Fprintf(stdOutOrErr, "Start a Stdio Tunnel Client or Server\nUsage of %s server | client\n  -help\n         output this help\n", os.Args[0])
	if isErr {
		os.Exit(2)
	}
}


func main() {
	if len(os.Args) < 2 {
		helpAndExit(true)
	}
	switch os.Args[1] {
	case subcommandKeyClient:
		stdiotunnel.Client(parseClientArgs(os.Args[1:]))
	case subcommandKeyServer:
	case subcommandKeyHelp:
		helpAndExit(false)
	default:
		helpAndExit(true)
	}
}
