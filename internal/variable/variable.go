package variable

import (
	"log"
	"math"
	"os"
	"os/user"
	"path"
)

var (
	// ConfigBaseDir - the project config dir
	ConfigBaseDir string
	// SSHHostKeyFileName - simple ssh ras private key file name
	SSHHostKeyFileName string = "ssh_host_rsa_key"
	// StdoutReadyTrigger - if stdiotunnel echo this string, then server ready
	StdoutReadyTrigger string = "::stdiotunnel-server-ready::"
	// MaxVirtualConnection - max virtual connection count
	MaxVirtualConnection = uint16(math.MaxUint16 - 1)
)

func init() {
	u, err := user.Current()
	if err != nil {
		log.Fatalf("Error: %s\n", err.Error())
		os.Exit(1)
	}
	ConfigBaseDir = path.Join(u.HomeDir, ".stdiotunnel")
}
