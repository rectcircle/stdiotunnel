package tools

import (
	"errors"
	"io/ioutil"
	"log"
	"net"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

// ToAddressString - return "$host:$port"
func ToAddressString(host string, port uint16) string {
	return net.JoinHostPort(host, strconv.FormatInt(int64(port), 10))
}

func getUserShellByPasswd(passwd string, username string) string {
	shell := ""
	for _, line := range strings.Split(passwd, "\n") {
		items := strings.Split(line, ":")
		if len(items) < 7 {
			continue
		}
		if items[0] == username {
			shell = items[6]
			break
		}
	}
	return shell
}

func getLinuxUserShell() string {
	bytes, err := ioutil.ReadFile("/etc/passwd")
	if err != nil {
		return ""
	}
	u, err2 := user.Current()
	if err2 != nil {
		return ""
	}
	passwd := string(bytes)
	return getUserShellByPasswd(passwd, u.Username)
}

func getDarwinUserShell() string {
	u, err2 := user.Current()
	if err2 != nil {
		return ""
	}
	output, err := exec.Command("/usr/bin/dscl", ".", "-read", u.HomeDir, "UserShell").Output()
	// output, err := exec.Command("ls").Output()
	if err != nil {
		return ""
	}
	return strings.Trim(strings.Split(string(output), ":")[1], " \t\n\r")
}

// GetUnixUserShell - get current user default shell
func GetUnixUserShell() string {
	var shell string = ""
	switch runtime.GOOS {
	case "darwin":
		shell = getDarwinUserShell()
	case "linux":
		shell = getLinuxUserShell()
	}
	if shell == "" {
		return "bash"
	}
	return shell
}

// PathExist - return whether exist of path
func PathExist(path string) bool {
	_, err := os.Stat(path)
	if err == nil {
		return true
	}
	if os.IsNotExist(err) {
		return false
	}
	return false
}

// ReadOrCreateFile - read from config file, and return the file content
// if path not exist, will create the path and call `f()` to write to the file.
func ReadOrCreateFile(path string, f func() []byte) ([]byte, error) {
	if PathExist(path) {
		content, err := ioutil.ReadFile(path)
		if err != nil {
			return nil, err
		}
		return content, nil
	}
	content := f()
	dir := filepath.Dir(path)
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		return nil, err
	}
	file, err2 := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err2 != nil {
		return nil, err2
	}
	file.Write(content)
	file.Close()
	return content, nil
}

// LogAndExitIfErr - will log and exit if err != nil
func LogAndExitIfErr(err error) {
	if err != nil {
		log.Fatalf("error: %s\n", err.Error())
		os.Exit(1)
	}
}

var stdinToChannelOnce sync.Once
var stdinChannel chan []byte

// StdinToChannel - get one same stdin channel
func StdinToChannel() <-chan []byte {
	stdinToChannelOnce.Do(func() {
		stdinChannel = make(chan []byte)
		go func() {
			var (
				buffer       = make([]byte, 4096, 4096)
				err    error = nil
				n            = int(0)
			)
			for {
				n, err = os.Stdin.Read(buffer)
				if err != nil {
					if err.Error() == "EOF" {
						LogAndExitIfErr(errors.New("stdin has eof, exit"))
					}
					LogAndExitIfErr(err)
				}
				copyBuffer := make([]byte, n, n)
				copy(copyBuffer, buffer[:n])
				stdinChannel <- copyBuffer
			}
		}()
	})
	return stdinChannel
}
