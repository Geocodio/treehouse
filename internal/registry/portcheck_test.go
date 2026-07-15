package registry

import "testing"

func TestParseLsofListeningPorts(t *testing.T) {
	// A representative sample of real lsof -nP -iTCP -sTCP:LISTEN output:
	// a header row, an IPv4 wildcard listener, an IPv4 loopback listener,
	// and an IPv6 listener, in the column layout lsof actually prints.
	output := []byte(`COMMAND   PID       USER   FD   TYPE             DEVICE SIZE/OFF NODE NAME
node    12345 codemonkey   23u  IPv4 0x1111111111      0t0  TCP *:4640 (LISTEN)
postgres 6789 codemonkey    7u  IPv4 0x2222222222      0t0  TCP 127.0.0.1:5432 (LISTEN)
redis-se 4321 codemonkey    6u  IPv6 0x3333333333      0t0  TCP [::1]:6379 (LISTEN)
`)

	ports := parseLsofListeningPorts(output)

	want := map[int]bool{4640: true, 5432: true, 6379: true}
	if len(ports) != len(want) {
		t.Fatalf("parseLsofListeningPorts() = %v, want %v", ports, want)
	}
	for port := range want {
		if !ports[port] {
			t.Errorf("parseLsofListeningPorts() missing port %d", port)
		}
	}
}

func TestParseLsofListeningPorts_EmptyOutput(t *testing.T) {
	ports := parseLsofListeningPorts([]byte(""))
	if len(ports) != 0 {
		t.Errorf("parseLsofListeningPorts(\"\") = %v, want empty", ports)
	}
}

func TestParseLsofListeningPorts_IgnoresNonListenLines(t *testing.T) {
	// Defensive: even if a non-LISTEN line somehow made it into the output
	// (it shouldn't, given -sTCP:LISTEN), it must not be parsed as a port.
	output := []byte("node 1 me 2u IPv4 0x1 0t0 TCP 10.0.0.1:4640->10.0.0.2:9999 (ESTABLISHED)\n")

	ports := parseLsofListeningPorts(output)
	if len(ports) != 0 {
		t.Errorf("parseLsofListeningPorts() = %v, want empty for a non-LISTEN line", ports)
	}
}
