module github.com/media-streaming-mesh/msm-dp

go 1.18

require (
	github.com/lucas-clemente/quic-go v0.28.1
	github.com/mengelbart/rtp-over-quic v0.0.0-20221101231535-ad471cda73af
	github.com/mengelbart/scream-go v0.4.1-0.20220916152424-a421761640a2
	github.com/pion/interceptor v0.1.12
	github.com/pion/rtcp v1.2.10
	github.com/pion/rtp v1.7.13
	github.com/sirupsen/logrus v1.9.0
	google.golang.org/grpc v1.48.0
	google.golang.org/protobuf v1.28.0

)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/francoispqt/gojay v1.2.13 // indirect
	github.com/fsnotify/fsnotify v1.5.1 // indirect
	github.com/go-task/slim-sprig v0.0.0-20210107165309-348f09dbbbc0 // indirect
	github.com/golang/mock v1.6.0 // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/marten-seemann/qtls-go1-18 v0.1.3 // indirect
	github.com/marten-seemann/qtls-go1-19 v0.1.1 // indirect
	github.com/niemeyer/pretty v0.0.0-20200227124842-a10e7caefd8e // indirect
	github.com/nxadm/tail v1.4.8 // indirect
	github.com/onsi/ginkgo v1.16.5 // indirect
	github.com/onsi/gomega v1.20.1 // indirect
	github.com/pion/logging v0.2.2 // indirect
	github.com/pion/randutil v0.1.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	golang.org/x/crypto v0.0.0-20220516162934-403b01795ae8 // indirect
	golang.org/x/exp v0.0.0-20220722155223-a9213eeb770e // indirect
	golang.org/x/mod v0.6.0-dev.0.20220419223038-86c51ed26bb4 // indirect
	golang.org/x/net v0.0.0-20220722155237-a158d28d115b // indirect
	golang.org/x/text v0.3.7 // indirect
	golang.org/x/tools v0.1.12 // indirect
	google.golang.org/genproto v0.0.0-20220519153652-3a47de7e79bd // indirect
	gopkg.in/check.v1 v1.0.0-20200227125254-8fa46927fb4f // indirect
	gopkg.in/tomb.v1 v1.0.0-20141024135613-dd632973f1e7 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

require (
	github.com/stretchr/testify v1.8.0
	golang.org/x/sys v0.0.0-20220909162455-aba9fc2a8ff2 // indirect
)

replace github.com/lucas-clemente/quic-go v0.28.1 => github.com/mengelbart/quic-go v0.7.1-0.20220916195640-e314b18dd0a4

replace github.com/pion/rtp v1.7.13 => github.com/mengelbart/rtp v1.7.14-0.20220728010821-271390af6fab
