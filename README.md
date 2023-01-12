# MSM RTP Proxy

This is a simple reference implementation of the MSM RTP Proxy Data Plane, written in Golang.   Not intended for production deployments as feature set is limited and as Garbage Collection is likely to cause packet drops.

It consists of two halves:

1. a gRPC/protobuf API server that receives flow setup messages from the MSM Controller
2. a simple RTP/RTCP proxy

All code is in a single module. and the RTP/RTCP proxy runs as two instances of a goroutine (one each for port 8050 and 8051).

To do:

1. Implement hash-map for multiple streams
2. verify support for multiple source ports from the same IP
3. add RTPoQUIC support to enable node-to-node streams
4. verify inbound/outbound external support for both interleaved and non-interleaved RTSP
