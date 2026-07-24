package diagnostics

import (
	"errors"
	"testing"

	"github.com/moby/moby/api/types/container"
)

func TestParseHostPortProtoDirect(t *testing.T) {
	tests := []struct {
		input         string
		expectedPort  string
		expectedProto string
	}{
		{"8080", "8080", "tcp"},
		{"8080/tcp", "8080", "tcp"},
		{"53/udp", "53", "udp"},
		{"80:8080/tcp", "80", "tcp"},
		{"127.0.0.1:8080", "127.0.0.1", "tcp"},
		{"0.0.0.0:80:8080/udp", "80", "udp"},
		{"[::1]:8080", "8080", "tcp"},
		{"[::1]:8080/udp", "8080", "udp"},
		{"[::]:80", "80", "tcp"},
		{"[::]:8080/udp", "8080", "udp"},
		{"[fe80::1]:8080:80/tcp", "8080", "tcp"},
		{"[::1]:8000-8005/tcp", "8000-8005", "tcp"},
		{"", "", "tcp"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			port, proto := ParseHostPortProto(tt.input)
			if port != tt.expectedPort || proto != tt.expectedProto {
				t.Errorf("ParseHostPortProto(%q) = (%q, %q), expected (%q, %q)",
					tt.input, port, proto, tt.expectedPort, tt.expectedProto)
			}
		})
	}
}

func TestGetOccupyingProcessMocking(t *testing.T) {
	origFunc := getOccupyingProcessFunc
	defer func() { getOccupyingProcessFunc = origFunc }()

	getOccupyingProcessFunc = func(port string, proto string) (string, int, error) {
		if port == "8080" {
			return "nginx", 1234, nil
		}
		return "", 0, errors.New("no occupying process found")
	}

	name, pid, err := GetOccupyingProcess("8080", "tcp")
	if err != nil || name != "nginx" || pid != 1234 {
		t.Errorf("expected nginx, 1234, nil; got %s, %d, %v", name, pid, err)
	}

	_, _, err = GetOccupyingProcess("9090", "tcp")
	if err == nil {
		t.Error("expected error for unoccupied port 9090, got nil")
	}
}

func TestIsPortBoundBySelf(t *testing.T) {
	containers := []container.Summary{
		{
			State: "running",
			Labels: map[string]string{
				"com.docker.compose.project": "myproj",
				"com.docker.compose.service": "web",
			},
			Ports: []container.PortSummary{
				{PublicPort: 8080, Type: "tcp"},
			},
		},
	}

	if !isPortBoundBySelf(8080, "tcp", containers, "myproj", "web") {
		t.Error("expected port 8080 to be bound by self")
	}

	if isPortBoundBySelf(9090, "tcp", containers, "myproj", "web") {
		t.Error("port 9090 should not be bound by self")
	}

	if isPortBoundBySelf(8080, "tcp", containers, "otherproj", "web") {
		t.Error("port 8080 under different project should not be bound by self")
	}
}
