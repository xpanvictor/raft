package main

import (
	"flag"
	"fmt"
	"github.com/xpanvictor/raft/commons"
	"github.com/xpanvictor/raft/configs"
	raft "github.com/xpanvictor/raft/node"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

var (
	done bool
	rw   = sync.Mutex{}
)

var (
	port        = flag.Int("port", 5000, "The starting port")
	serverCount = flag.Int("server_count", 5, "The amount of servers to deploy")
)

func main() {
	flag.Parse()
	log.Printf("Starting server")
	ch := make(chan os.Signal)
	// will create  nodes
	for i := 0; i < *serverCount; i++ {
		go raft.NewNode(commons.NodeID(i), configs.ELECTION_TIMEOUT, commons.Port(*port+i), ch)
	}
	signal.Notify(ch, syscall.SIGINT)
	signal.Notify(ch, syscall.SIGKILL)
	caughtSignal := <-ch
	log.Printf("Closing raft with %v", caughtSignal)
}

func periodic() {
	// infinite loop that waits till done
	for {
		time.Sleep(1 * time.Second)
		fmt.Println("Heartbeat")
		rw.Lock()
		if done {
			return
		}
		rw.Unlock()
	}
}
