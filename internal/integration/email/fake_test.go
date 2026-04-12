package email_test

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

type capturedMail struct {
	authAttempted bool
	data          string
}

// startFakeSMTP starts a minimal SMTP server on a random localhost port.
// When advertiseAuth is true the server offers AUTH PLAIN in the EHLO response
// and records whether the client used it.
// Returns host, port, and a getter for the captured mail (safe to call after
// the Send method returns, because smtp.SendMail is synchronous).
func startFakeSMTP(t *testing.T, advertiseAuth bool) (host string, port int, get func() capturedMail) {
	t.Helper()

	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)

	var mu sync.Mutex
	var captured capturedMail

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()

		r := bufio.NewReader(conn)
		w := bufio.NewWriter(conn)

		writeLine := func(s string) {
			_, _ = fmt.Fprintf(w, "%s\r\n", s)
			_ = w.Flush()
		}
		readLine := func() string {
			line, _ := r.ReadString('\n')

			return strings.TrimRight(line, "\r\n")
		}

		writeLine("220 localhost SMTP ready")

		readLine() // EHLO
		if advertiseAuth {
			writeLine("250-localhost Hello")
			writeLine("250 AUTH PLAIN LOGIN")
		} else {
			writeLine("250 localhost Hello")
		}

		next := readLine()
		if strings.HasPrefix(next, "AUTH") {
			mu.Lock()
			captured.authAttempted = true
			mu.Unlock()
			writeLine("235 Authentication successful")
			next = readLine()
		}

		// MAIL FROM
		if strings.HasPrefix(next, "MAIL FROM") {
			writeLine("250 OK")
		}

		// RCPT TO
		readLine()
		writeLine("250 OK")

		// DATA
		readLine()
		writeLine("354 Start input, end with <CRLF>.<CRLF>")

		var lines []string
		for {
			line := readLine()
			if line == "." {
				break
			}
			// smtp protocol doubles a leading dot; undo that
			if strings.HasPrefix(line, "..") {
				line = line[1:]
			}
			lines = append(lines, line)
		}
		mu.Lock()
		captured.data = strings.Join(lines, "\n")
		mu.Unlock()
		writeLine("250 OK")

		readLine() // QUIT
		writeLine("221 Bye")
	}()

	t.Cleanup(func() { _ = ln.Close() })

	tcpAddr, ok := ln.Addr().(*net.TCPAddr)
	require.True(t, ok)

	return tcpAddr.IP.String(), tcpAddr.Port, func() capturedMail {
		mu.Lock()
		defer mu.Unlock()

		return captured
	}
}
