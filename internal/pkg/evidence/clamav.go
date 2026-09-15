package evidence

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"
)

const clamAVChunkSize = 64 << 10

// ClamAVScanner implements clamd's length-prefixed INSTREAM protocol. It does
// not shell out, write command arguments, or expose the scanner's raw response
// outside this package.
type ClamAVScanner struct {
	network string
	address string
	timeout time.Duration
	dialer  net.Dialer
}

func NewClamAVScanner(network, address string, timeout time.Duration) (*ClamAVScanner, error) {
	network = strings.TrimSpace(network)
	address = strings.TrimSpace(address)
	if network != "tcp" && network != "unix" {
		return nil, errors.New("clamav network must be tcp or unix")
	}
	if address == "" {
		return nil, errors.New("clamav address is required")
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &ClamAVScanner{network: network, address: address, timeout: timeout}, nil
}

func (s *ClamAVScanner) Scan(ctx context.Context, data io.Reader) (ScanReport, error) {
	if data == nil {
		return ScanReport{}, errors.New("scan data is required")
	}
	connection, err := s.dialer.DialContext(ctx, s.network, s.address)
	if err != nil {
		return ScanReport{}, fmt.Errorf("connect to malware scanner: %w", err)
	}
	defer connection.Close()
	stopCancellation := context.AfterFunc(ctx, func() { _ = connection.Close() })
	defer stopCancellation()
	deadline := time.Now().Add(s.timeout)
	if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
		deadline = contextDeadline
	}
	if err := connection.SetDeadline(deadline); err != nil {
		return ScanReport{}, fmt.Errorf("set malware scanner deadline: %w", err)
	}
	if err := writeAll(connection, []byte("zINSTREAM\x00")); err != nil {
		return ScanReport{}, fmt.Errorf("start malware scan: %w", err)
	}

	buffer := make([]byte, clamAVChunkSize)
	length := make([]byte, 4)
	emptyReads := 0
	for {
		if err := ctx.Err(); err != nil {
			return ScanReport{}, err
		}
		count, readErr := data.Read(buffer)
		if count < 0 || count > clamAVChunkSize {
			return ScanReport{}, errors.New("malware scan reader returned an invalid chunk size")
		}
		if count == 0 && readErr == nil {
			emptyReads++
			if emptyReads >= 100 {
				return ScanReport{}, io.ErrNoProgress
			}
		} else {
			emptyReads = 0
		}
		if count > 0 {
			binary.BigEndian.PutUint32(length, uint32(count))
			if err := writeAll(connection, length); err != nil {
				return ScanReport{}, fmt.Errorf("send malware scan chunk length: %w", err)
			}
			if err := writeAll(connection, buffer[:count]); err != nil {
				return ScanReport{}, fmt.Errorf("send malware scan chunk: %w", err)
			}
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return ScanReport{}, fmt.Errorf("read malware scan input: %w", readErr)
		}
	}
	if err := writeAll(connection, []byte{0, 0, 0, 0}); err != nil {
		return ScanReport{}, fmt.Errorf("finish malware scan stream: %w", err)
	}

	response, err := bufio.NewReader(io.LimitReader(connection, 4<<10)).ReadString(0)
	if err != nil && !errors.Is(err, io.EOF) {
		return ScanReport{}, fmt.Errorf("read malware scan result: %w", err)
	}
	response = strings.TrimSpace(strings.TrimSuffix(response, "\x00"))
	const prefix = "stream: "
	if !strings.HasPrefix(response, prefix) {
		return ScanReport{}, errors.New("malware scanner returned an invalid response")
	}
	result := strings.TrimPrefix(response, prefix)
	if result == "OK" {
		return ScanReport{Verdict: ScanClean, Engine: "clamav"}, nil
	}
	if strings.HasSuffix(result, " FOUND") {
		signature := strings.TrimSuffix(result, " FOUND")
		return ScanReport{Verdict: ScanInfected, Engine: "clamav", Signature: sanitizeSignature(signature)}, nil
	}
	return ScanReport{}, errors.New("malware scanner could not determine a verdict")
}

// HealthCheck verifies protocol-level scanner readiness without uploading a
// file. It is safe to use from private readiness and diagnostics probes.
func (s *ClamAVScanner) HealthCheck(ctx context.Context) error {
	connection, err := s.dialer.DialContext(ctx, s.network, s.address)
	if err != nil {
		return fmt.Errorf("connect to malware scanner: %w", err)
	}
	defer connection.Close()
	stopCancellation := context.AfterFunc(ctx, func() { _ = connection.Close() })
	defer stopCancellation()
	deadline := time.Now().Add(s.timeout)
	if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
		deadline = contextDeadline
	}
	if err := connection.SetDeadline(deadline); err != nil {
		return fmt.Errorf("set malware scanner deadline: %w", err)
	}
	if err := writeAll(connection, []byte("zPING\x00")); err != nil {
		return fmt.Errorf("ping malware scanner: %w", err)
	}
	response, err := bufio.NewReader(io.LimitReader(connection, 64)).ReadString(0)
	if err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("read malware scanner ping: %w", err)
	}
	if strings.TrimSpace(strings.TrimSuffix(response, "\x00")) != "PONG" {
		return errors.New("malware scanner returned an invalid ping response")
	}
	return nil
}

func writeAll(writer io.Writer, value []byte) error {
	for len(value) > 0 {
		written, err := writer.Write(value)
		if err != nil {
			return err
		}
		if written == 0 {
			return io.ErrShortWrite
		}
		value = value[written:]
	}
	return nil
}
