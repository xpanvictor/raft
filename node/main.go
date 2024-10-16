package raft

import (
	"context"
	"fmt"
	"github.com/xpanvictor/raft/commons"
	pb "github.com/xpanvictor/raft/protoc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"log"
	"net"
	"net/url"
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
	addr        commons.Addr
	// volatile state
	lastCommited commons.Index
	lastApplied  commons.Index
	// ticker
	ticker time.Ticker
}

type server struct {
	pb.UnimplementedRaftServiceServer
	nd *Node
}

func (s *server) RequestVote(_ context.Context, in *pb.VoteRequest) (*pb.VoteResponse, error) {
	nd := s.nd
	if nd == nil {
		return nil, fmt.Errorf("node not found")
	}
	log.Printf("Node %v: Got a request from node %v", nd.id, in.CandidateId)
	vote := nd.checkVote(in)
	log.Printf("Node %v: processed from  node %v", nd.id, in.CandidateId)
	return &pb.VoteResponse{
		CurrentTerm: 0,
		VoteGranted: vote,
	}, nil
}

func (n *Node) checkVote(vr *pb.VoteRequest) bool {
	if n.currentTerm > commons.Term(vr.Term) {
		// if node votedFor is nil or candidate id, proceed
		cid := commons.NodeID(vr.CandidateId)
		if n.votedFor == 0 || n.votedFor == cid {
			// check if log is up to date
			if n.lastCommited <= commons.Index(vr.LastLogIndex) {
				n.votedFor = cid
				return true
			}
		}
	}
	return false
}

func NewNode(id commons.NodeID, actionTimeout time.Duration, addr string, ch chan os.Signal) *Node {
	a, err := url.Parse(addr)
	if err != nil {
		log.Fatalf("Cannot parse addr %v", err)
	}
	nd := &Node{
		id:           id,
		currentTerm:  0,
		votedFor:     0,
		logs:         nil,
		addr:         commons.Addr(a.String()),
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
	lis, err := net.Listen("tcp", n.addr.GetHost())
	if err != nil {
		log.Fatalf("Node failed to listen, %v", err)
	}
	s := grpc.NewServer()
	defer s.Stop()

	log.Printf("Listener at %v", lis.Addr().Network())

	pb.RegisterRaftServiceServer(s, &server{nd: n})
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
			// TODO: Declare candidateship or send heartbeat
			n.declareCandidate()
			log.Printf("Ticker ticked")
		case sig := <-quit:
			log.Printf("Node got call %s", sig)
			return // track signal later
		}
	}
}

func (n *Node) declareCandidate() {
	// say address is mine + 1

	nextPort := 5001
	conn, err := grpc.NewClient(fmt.Sprintf(":%d", nextPort), grpc.WithTransportCredentials(insecure.NewCredentials()))
	log.Printf("Calling %d", nextPort)
	if err != nil {
		log.Fatalf("Can't send connection")
	}
	defer conn.Close()

	c := pb.NewRaftServiceClient(conn)

	// make context for reqs
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	// contact addr
	lastLogIndex := len(n.logs) - 1

	// update currentTerm and request vote
	n.currentTerm += 1

	d, err := c.RequestVote(ctx, &pb.VoteRequest{
		Term:         int32(n.currentTerm),
		CandidateId:  int32(n.id),
		LastLogTerm:  0, // int32((*n.logs[lastLogIndex]).term)
		LastLogIndex: int32(lastLogIndex),
	})
	if err != nil || d == nil {
		log.Printf("Error declaring node: %d as leader, %v", n.id, err)
		return
	}

	log.Printf("Response, %d, voted: %v", d.CurrentTerm, d.VoteGranted)
	log.Println("Move")
}
