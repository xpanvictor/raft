package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/xpanvictor/raft/commons"
	"google.golang.org/grpc"
	"log"
	"net"
	"sync"
	"time"

	pb "github.com/xpanvictor/raft/protoc"
)

var done bool
var rw sync.Mutex

// LogEntry Each entry in a log
type LogEntry struct {
	term     commons.Term
	commands []*commons.Command
}

// Node The Node's state
type Node struct {
	id          commons.NodeID
	currentTerm commons.Term
	votedFor    commons.NodeID
	logs        []*LogEntry
	port        commons.Port
	// volatile state
	lastCommited commons.Index
	lastApplied  commons.Index
}

type server struct {
	pb.UnimplementedRaftServiceServer
}

func (s *server) RequestVote(_ context.Context, in *pb.VoteRequest) (*pb.VoteResponse, error) {
	return &pb.VoteResponse{
		CurrentTerm: 0,
		VoteGranted: false,
	}, nil
}

var (
	port         = flag.Int("port", 5000, "The starting port")
	server_count = flag.Int("server_count", 5, "The amount of servers to deploy")
)

// manage and spun up servers
func (n *Node) startNode(port commons.Port) {
	// start the grpc server
	// listen for tcp
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", n.port))
	if err != nil {
		log.Fatalf("Node failed to listen, %v", err)
	}
	s := grpc.NewServer()
	pb.RegisterRaftServiceServer(s, &server{})
	log.Printf("Server listening at %v", lis.Addr())
	// run server
	if err := s.Serve(lis); err != nil {
		log.Fatalf("Can't serve listener, %v", err)
	}
}

func main() {
	flag.Parse()

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
