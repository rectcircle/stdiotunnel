package main

import "testing"

func Test_parseArgs(t *testing.T) {
	type args struct {
		subcommand string
		desc       string
		args       []string
	}
	tests := []struct {
		name     string
		args     args
		wantHost string
		wantPort uint16
	}{
		// Case1
		{
			name: "test server help",
			args: args{
				subcommand: "server",
				desc:       "run a echo server",
				args:       []string{"server"},
			},
			wantHost: "127.0.0.1",
			wantPort: 7,
		},
		// Case2
		{
			name: "test server help",
			args: args{
				subcommand: "server",
				desc:       "run a echo server",
				args:       []string{"server", "-h", "0.0.0.0", "-p", "10007"},
			},
			wantHost: "0.0.0.0",
			wantPort: 10007,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotHost, gotPort := parseArgs(tt.args.subcommand, tt.args.desc, tt.args.args)
			if gotHost != tt.wantHost {
				t.Errorf("parseArgs() gotHost = %v, want %v", gotHost, tt.wantHost)
			}
			if gotPort != tt.wantPort {
				t.Errorf("parseArgs() gotPort = %v, want %v", gotPort, tt.wantPort)
			}
		})
	}
}
