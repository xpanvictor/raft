package commons

import (
	"log"
	"net/url"
)

type Addr string
type Command string
type Port int32
type Term int32
type Index int32
type NodeID int32

func (addr *Addr) GetHost() string {
	parsedAddr, err := url.Parse(string(*addr))
	if err != nil {
		log.Fatalf("Unable to parse addr %v", err)
	}
	return parsedAddr.Host
}
