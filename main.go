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
	nodesUrlFile = flag.String("nodes_url_file", "nodes.txt", "The file containing all nodes url")
	newClient    = flag.Bool("new_client", false, "To just add another client to a running system")
)

func main() {
	flag.Parse()
	log.Printf("Starting server")
	ch := make(chan os.Signal)
	// will create  nodes
	// creates the nodesUrlFile
	file, err := os.OpenFile(*nodesUrlFile, os.O_RDWR, 0644)
	if err != nil {
		log.Fatalf("Unable to create nodes url file %v", err)
	}
	defer file.Close()
	// Use the seed nodes to initiate the system
	seedNodes := commons.ArrStrFromFile(file)
	for i, addr := range seedNodes {
		raft.NewNode(commons.NodeID(i), configs.ELECTION_TIMEOUT, addr, ch)
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
