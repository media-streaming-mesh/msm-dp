package proxy

import (
	"log"
	"net"
	"time"
)

type udpWrite struct {
	addr *net.UDPAddr
	buf  []byte
}

type serverUdpListener struct {
	p     *program
	nconn *net.UDPConn
	flow  trackFlow
	write chan *udpWrite
	done  chan struct{}
}

func newServerUdpListener(p *program, port int, flow trackFlow) (*serverUdpListener, error) {
	nconn, err := net.ListenUDP("udp", &net.UDPAddr{
		Port: port,
	})
	if err != nil {
		return nil, err
	}

	l := &serverUdpListener{
		p:     p,
		nconn: nconn,
		flow:  flow,
		write: make(chan *udpWrite),
		done:  make(chan struct{}),
	}

	l.log("opened on :%d", port)
	return l, nil
}

func (l *serverUdpListener) log(format string, args ...interface{}) {
	var label string
	if l.flow == _TRACK_FLOW_RTP {
		label = "RTP"
	} else {
		label = "RTCP"
	}
	log.Printf("[UDP/"+label+" listener] "+format, args...)
}

func (l *serverUdpListener) run() {
	go func() {
		for w := range l.write {
			l.nconn.SetWriteDeadline(time.Now().Add(l.p.args.writeTimeout))
			l.nconn.WriteTo(w.buf, w.addr)

			log.Printf("UDP Write %v %v", w.buf, w.addr)
		}
	}()

	for {
		// create a buffer for each read.
		// this is necessary since the buffer is propagated with channels
		// so it must be unique.
		buf := make([]byte, 2048) // UDP MTU is 1400
		n, addr, err := l.nconn.ReadFromUDP(buf)
		if err != nil {
			break
		}
		func() {
			l.p.tcpl.mutex.RLock()
			defer l.p.tcpl.mutex.RUnlock()

			// find path and track id from ip and port
			path, trackId := func() (string, int) {
				for _, pub := range l.p.tcpl.publishers {
					for i, t := range pub.streamTracks {
						if !pub.ip().Equal(addr.IP) {
							continue
						}
						log.Printf("pub.ip, addr.ip %v %v", n, addr)

						if l.flow == _TRACK_FLOW_RTP {
							if t.rtpPort == addr.Port {
								log.Printf("_TRACK_FLOW_RTP === Track ID, ip port ==>  %v %v", pub.path, i)
								return pub.path, i
							}
						} else {
							if t.rtcpPort == addr.Port {
								log.Printf("_TRACK_FLOW_RTCP === Track ID, ip port ==>  %v %v", pub.path, i)
								return pub.path, i
							}
						}
					}
				}
				return "", -1
			}()
			log.Printf("path, trackID %v %v", path, trackId)

			if path == "" {
				return
			}

			l.p.tcpl.forwardTrack(path, trackId, l.flow, buf[:n])
			log.Printf("UDP Logs tcpl.forwardTrack -> %v %v %v %v", path, trackId, l.flow, buf[:n])
		}()
	}

	close(l.write)

	close(l.done)
}

func (l *serverUdpListener) close() {
	l.nconn.Close()
	<-l.done
}
