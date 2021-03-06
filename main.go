package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"

	zmq "github.com/pebbe/zmq4"
)

const (
	NF5HeaderLen = 24
	NF5RecordLen = 48
)

func uint32toip(a uint32) string {
	return fmt.Sprintf("%d.%d.%d.%d", byte(a>>24), byte(a>>16), byte(a>>8), byte(a))
}

type NF5Header struct {
	Version      uint16
	Count        uint16
	SysUpTime    uint32
	EpochSeconds uint32
	Nanoseconds  uint32
	FlowsSeen    uint32
	EngineType   byte
	EngineID     byte
	SamplingInfo uint16
}
type NF5Record struct {
	SourceIPaddr             uint32
	Destination              uint32
	Nexthop                  uint32
	InboundsnmpIFindex       uint16
	OutboundsnmpIFindex      uint16
	PacketCount              uint32
	ByteCount                uint32
	TimeatStart              uint32
	TimeatEnd                uint32
	SourcePort               uint16
	DestinationPort          uint16
	Onepadbyte               byte
	TCPflags                 byte
	Layer4Protocol           byte
	ToS                      byte
	SourceAS                 uint16
	DestAS                   uint16
	SourceMaskBitsCount      byte
	DestinationMaskBitsCount byte
	TwoPadBytes              uint16
}
type NF5json struct {
	SourceIPaddr      string
	DestinationIPaddr string
	ByteCount         uint32
	PacketCount       uint32
	SourcePort        uint16
	DestinationPort   uint16
	TCPflags          byte
	Layer4Protocol    byte
	ToS               byte
}

func (r *NF5json) serialize() ([]byte, error) {
	t, err := json.Marshal(r)
	if err != nil {
		return t, err
	}
	return t, nil

}
func handlePacket(in chan []byte, out chan NF5Record) {
	var (
		header NF5Header
		record NF5Record
		offset uint16
		i      uint16
	)
	for buf := range in {
		offset = 0
		buffer := bytes.NewReader(buf[:NF5HeaderLen])
		err := binary.Read(buffer, binary.BigEndian, &header)
		if err == nil {
			if header.Version == 5 {
				buf = buf[NF5HeaderLen:]
				for i = 1; i <= header.Count; i += 1 {
					buffer = bytes.NewReader(buf[offset : NF5RecordLen*i])
					e := binary.Read(buffer, binary.BigEndian, &record)
					if e == nil {
						out <- record
					}
					offset = NF5RecordLen * i
				}
			}
		}
	}
}
func zmqpublisher(socket *zmq.Socket, ch chan NF5Record) {
	for {
		r := <-ch
		sip := uint32toip(r.SourceIPaddr)
		dip := uint32toip(r.Destination)
		nf5 := NF5json{SourceIPaddr: sip,
			SourcePort:        r.SourcePort,
			DestinationIPaddr: dip,
			DestinationPort:   r.DestinationPort,
			ByteCount:         r.ByteCount,
			PacketCount:       r.PacketCount,
			ToS:               r.ToS,
			TCPflags:          r.TCPflags,
			Layer4Protocol:    r.Layer4Protocol}
		ret, err := nf5.serialize()
		if err == nil {
			socket.Send(string(ret), 0)
		}
	}
}
func main() {
	var (
		zmqbind string
	)
	udpPort := flag.Int("port", 12000, "Listen UDP port")
	workers := flag.Int("workers", 128, "Count of workers")
	flag.StringVar(&zmqbind, "zmq", "tcp://*:5557", "ZeroMQ bind parameter")
	flag.Parse()
	addr, _ := net.ResolveUDPAddr("udp", fmt.Sprintf(":%d", *udpPort))
	sock, err := net.ListenUDP("udp", addr)
	if err != nil {
		log.Fatalln(err)
	}
	sock.SetReadBuffer(1048576)
	out := make(chan NF5Record)
	in := make(chan []byte)
	socket, _ := zmq.NewSocket(zmq.PUB)
	defer socket.Close()
	err = socket.Bind(zmqbind)
	if err != nil {
		log.Fatalln("Unable to start 0MQ socket", err)
	}
	log.Println("Started")
	go zmqpublisher(socket, out)
	for i := 0; i < *workers; i++ {
		go handlePacket(in, out)
	}
	buf := make([]byte, 1500)
	for {
		_, _, err := sock.ReadFromUDP(buf)
		if err != nil {
			fmt.Println(err)
			continue
		}
		in <- buf
	}
}
