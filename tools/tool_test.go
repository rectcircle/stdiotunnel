package tools

import (
	"fmt"
	"math/rand"
	"os"
	"os/user"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
	"time"
)

func Test_getDarwinUserShell(t *testing.T) {
	fmt.Println(getDarwinUserShell())
}

func Test_getUserShellByPasswd(t *testing.T) {
	type args struct {
		passwd   string
		username string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "Linux get user shell",
			args: args{
				passwd:   "user:x:1001:1001::/home/user:/bin/zsh\nroot:x:0:0:root:/root:/bin/zsh\n",
				username: "user",
			},
			want: "/bin/zsh",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getUserShellByPasswd(tt.args.passwd, tt.args.username); got != tt.want {
				t.Errorf("getUserShellByPasswd() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPathExist(t *testing.T) {
	type args struct {
		path string
	}
	u, _ := user.Current()
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "Home exist",
			args: args{
				path: u.HomeDir,
			},
			want: true,
		},
		{
			name: "os.Args[0] exist",
			args: args{
				path: os.Args[0],
			},
			want: true,
		},
		{
			name: "/qazwsxedc ",
			args: args{
				path: "/qazwsxedc",
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := PathExist(tt.args.path); got != tt.want {
				t.Errorf("PathExist() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestReadOrCreateFile(t *testing.T) {
	type args struct {
		path string
		f    func() []byte
	}
	rand.Seed(time.Now().UnixNano())
	tmpPath := filepath.Join(os.TempDir(), strconv.FormatUint(uint64(rand.Uint32()), 10))
	content := []byte("abc")
	tests := []struct {
		name    string
		args    args
		want    []byte
		wantErr bool
	}{
		{
			name: "tmp file test1",
			args: args {
				path: tmpPath,
				f: func() []byte { return content },
			},
			want: content,
			wantErr: false,
		},
		{
			name: "tmp file test2",
			args: args {
				path: tmpPath,
				f: func() []byte { return content },
			},
			want: content,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ReadOrCreateFile(tt.args.path, tt.args.f)
			if (err != nil) != tt.wantErr {
				t.Errorf("ReadOrCreateConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ReadOrCreateConfig() = %v, want %v", got, tt.want)
			}
		})
	}
}
