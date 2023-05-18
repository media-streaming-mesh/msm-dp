package main

import (
	"fmt"
	"io"
	"net"
	"testing"
)

// Note: Not starting the server in the goroutine,
// as the test case is likely to be finished before the listener has run the/its test.
func TestConn(t *testing.T) {
	message := "Hi there!\n"

	go func() {
		conn, err := net.Dial("tcp", "127.0.0.1:8050")
		if err != nil {
			t.Error(err)
			return
		}
		defer func(conn net.Conn) {
			err := conn.Close()
			if err != nil {
				t.Error(err)
			}
		}(conn)

		if _, err := fmt.Fprintf(conn, message); err != nil {
			t.Error(err)
			return
		}
	}()

	l, err := net.Listen("tcp", "127.0.0.1:8050")
	if err != nil {
		t.Fatal(err)
	}
	defer func(l net.Listener) {
		err := l.Close()
		if err != nil {
			t.Fatal(err)
		}
	}(l)
	for {
		conn, err := l.Accept()
		if err != nil {
			return
		}
		// ignore resource leak warning.
		// We need "defer" to keep the connection open
		// else we will get "use of closed network connection" error
		defer func(conn net.Conn) {
			err := conn.Close()
			if err != nil {
				t.Fatal(err)
			}
		}(conn)

		buf, err := io.ReadAll(conn)
		if err != nil {
			t.Fatal(err)
		}

		fmt.Println(string(buf[:]))
		if msg := string(buf[:]); msg != message {
			t.Fatalf("Unexpected message:\nGot:\t\t%s\nExpected:\t%s\n", msg, message)
		}
		return // Done
	}
}
