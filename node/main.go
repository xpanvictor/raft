package raft

import (
	"context"
	"fmt"
	"github.com/xpanvictor/raft/commons"
	pb "github.com/xpanvictor/raft/protoc"
	"google.golang.org/grpc"
	"log"
	"net"
	"os"
	"sync"
	"time"
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
	// ticker
	ticker time.Ticker
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

func NewNode(id commons.NodeID, actionTimeout time.Duration, port commons.Port, ch chan os.Signal) *Node {
	nd := &Node{
		id:           id,
		currentTerm:  0,
		votedFor:     0,
		logs:         nil,
		port:         port,
		lastCommited: 0,
		lastApplied:  0,
		ticker:       *time.NewTicker(actionTimeout),
	}

	// start server and operations
	go nd.startNode(ch)
	go nd.operate(ch)

	return nd
}

// manage and spin up server
func (n *Node) startNode(quit chan os.Signal) {
	// start the grpc server
	// listen for tcp
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", n.port))
	if err != nil {
		log.Fatalf("Node failed to listen, %v", err)
	}
	s := grpc.NewServer()
	defer s.Stop()

	pb.RegisterRaftServiceServer(s, &server{})
	log.Printf("GRPC Server listening at %v", lis.Addr())
	// run server
	if err := s.Serve(lis); err != nil {
		log.Fatalf("Can't serve listener, %v", err)
	}
	// delay till signal
	signal := <-quit
	log.Printf("Closing grpc server: %v", signal)
}

// start operations
func (n *Node) operate(quit chan os.Signal) {
	// waits every election count which can be reset
	// constantly run
	log.Printf("Running node: %d", n.id)
	for {
		select {
		case <-n.ticker.C:
			// TODO: here is where Leader election starts
			log.Printf("Ticker ticked")
		case sig := <-quit:
			log.Printf("Node got call %s", sig)
			return // track signal later
		}
	}
}
