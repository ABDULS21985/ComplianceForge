package evidence

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

func TestClamAVScannerINSTREAMProtocol(t *testing.T) {
	tests := []struct {
		name          string
		response      string
		wantVerdict   ScanVerdict
		wantSignature string
		wantError     bool
	}{
		{name: "clean", response: "stream: OK\x00", wantVerdict: ScanClean},
		{name: "infected", response: "stream: Eicar-Test-Signature FOUND\x00", wantVerdict: ScanInfected, wantSignature: "Eicar-Test-Signature"},
		{name: "invalid", response: "unexpected\x00", wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatalf("Listen() error = %v", err)
			}
			defer listener.Close()
			payload := bytes.Repeat([]byte("evidence"), 20_000)
			serverError := make(chan error, 1)
			go func() {
				connection, acceptErr := listener.Accept()
				if acceptErr != nil {
					serverError <- acceptErr
					return
				}
				defer connection.Close()
				command := make([]byte, len("zINSTREAM\x00"))
				if _, readErr := io.ReadFull(connection, command); readErr != nil {
					serverError <- readErr
					return
				}
				if string(command) != "zINSTREAM\x00" {
					serverError <- errors.New("invalid INSTREAM command")
					return
				}
				var received bytes.Buffer
				length := make([]byte, 4)
				for {
					if _, readErr := io.ReadFull(connection, length); readErr != nil {
						serverError <- readErr
						return
					}
					count := binary.BigEndian.Uint32(length)
					if count == 0 {
						break
					}
					if _, readErr := io.CopyN(&received, connection, int64(count)); readErr != nil {
						serverError <- readErr
						return
					}
				}
				if !bytes.Equal(received.Bytes(), payload) {
					serverError <- errors.New("scanner server received different payload")
					return
				}
				_, serverWriteErr := io.WriteString(connection, test.response)
				serverError <- serverWriteErr
			}()

			scanner, err := NewClamAVScanner("tcp", listener.Addr().String(), 2*time.Second)
			if err != nil {
				t.Fatalf("NewClamAVScanner() error = %v", err)
			}
			report, err := scanner.Scan(context.Background(), bytes.NewReader(payload))
			if test.wantError != (err != nil) {
				t.Fatalf("Scan() error = %v, wantError=%t", err, test.wantError)
			}
			if report.Verdict != test.wantVerdict || report.Signature != test.wantSignature || (!test.wantError && report.Engine != "clamav") {
				t.Fatalf("Scan() report = %#v", report)
			}
			if err := <-serverError; err != nil {
				t.Fatalf("scanner server error = %v", err)
			}
		})
	}
}

func TestClamAVScannerValidationAndCancellation(t *testing.T) {
	for _, input := range []struct{ network, address string }{{"udp", "host"}, {"tcp", ""}} {
		if _, err := NewClamAVScanner(input.network, input.address, time.Second); err == nil {
			t.Fatalf("NewClamAVScanner(%q, %q) error = nil", input.network, input.address)
		}
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	defer listener.Close()
	accepted := make(chan struct{})
	go func() {
		connection, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		defer connection.Close()
		close(accepted)
		_, _ = io.Copy(io.Discard, connection)
	}()
	scanner, _ := NewClamAVScanner("tcp", listener.Addr().String(), 10*time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, scanErr := scanner.Scan(ctx, strings.NewReader("evidence"))
		done <- scanErr
	}()
	<-accepted
	cancel()
	select {
	case scanErr := <-done:
		if scanErr == nil {
			t.Fatal("Scan() error = nil after cancellation")
		}
	case <-time.After(time.Second):
		t.Fatal("Scan() did not stop promptly after cancellation")
	}
}

func TestClamAVScannerHealthCheck(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	defer listener.Close()
	serverError := make(chan error, 1)
	go func() {
		connection, acceptErr := listener.Accept()
		if acceptErr != nil {
			serverError <- acceptErr
			return
		}
		defer connection.Close()
		command := make([]byte, len("zPING\x00"))
		if _, readErr := io.ReadFull(connection, command); readErr != nil {
			serverError <- readErr
			return
		}
		if string(command) != "zPING\x00" {
			serverError <- errors.New("unexpected ping command")
			return
		}
		_, writeErr := io.WriteString(connection, "PONG\x00")
		serverError <- writeErr
	}()
	scanner, err := NewClamAVScanner("tcp", listener.Addr().String(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := scanner.HealthCheck(context.Background()); err != nil {
		t.Fatalf("HealthCheck() error = %v", err)
	}
	if err := <-serverError; err != nil {
		t.Fatalf("server error = %v", err)
	}
}

type invalidClamAVReader struct{ count int }

func (r invalidClamAVReader) Read([]byte) (int, error) { return r.count, nil }

func TestClamAVScannerRejectsInvalidLengthsAndNoProgressWithoutChunkWrites(t *testing.T) {
	for _, count := range []int{-1, clamAVChunkSize + 1, 0} {
		t.Run(fmt.Sprintf("reader count %d", count), func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			received := make(chan []byte, 1)
			go func() {
				connection, err := listener.Accept()
				if err != nil {
					received <- nil
					return
				}
				defer connection.Close()
				_ = connection.SetDeadline(time.Now().Add(2 * time.Second))
				data, _ := io.ReadAll(io.LimitReader(connection, 32))
				received <- data
			}()
			scanner, err := NewClamAVScanner("tcp", listener.Addr().String(), 2*time.Second)
			if err != nil {
				t.Fatal(err)
			}
			report, err := scanner.Scan(context.Background(), invalidClamAVReader{count: count})
			if err == nil || report.Verdict != "" || (count == 0 && !errors.Is(err, io.ErrNoProgress)) {
				t.Fatalf("malformed reader reached verdict report=%+v error=%v", report, err)
			}
			if data := <-received; string(data) != "zINSTREAM\x00" {
				t.Fatalf("malformed reader emitted a length/chunk: %q", data)
			}
		})
	}
}
