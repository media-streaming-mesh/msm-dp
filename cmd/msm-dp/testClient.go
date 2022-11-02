package main

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"github.com/lucas-clemente/quic-go"
	quick "github.com/media-streaming-mesh/msm-dp/cmd/quic"
	"strconv"
	"time"
)

func main() {
	startClient()
}
func startClient() {
	quicConfig := &quic.Config{
		MaxIdleTimeout: time.Minute,
	}
	tlsConf := &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{"quick"},
	}
	conn, err := quick.Dial("127.0.0.1:8050", tlsConf, quicConfig)
	if err != nil {
		panic(err)
	}

	for i := 0; ; i++ {
		message := "Ping from client #" + strconv.Itoa(i)
		fmt.Fprintf(conn, message+"\n")
		fmt.Printf("Sending message: %s\n", message)
		// listen for reply
		answer, err := bufio.NewReader(conn).ReadString('\n')
		if err != nil {
			panic(err)
		}
		fmt.Print("Message from server: " + answer)
		time.Sleep(time.Second)
	}
}
