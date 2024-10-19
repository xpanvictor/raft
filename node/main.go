package raft

import (
	"context"
	"fmt"
	"github.com/xpanvictor/raft/commons"
	"github.com/xpanvictor/raft/configs"
	pb "github.com/xpanvictor/raft/protoc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"log"
	"net"
	"net/url"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

var done bool

// LogEntry Each entry in a log
type LogEntry struct {
	term     commons.Term
	commands []*commons.Command
}

type NodeState int

const (
	FOLLOWER NodeState = iota
	CANDIDATE
	LEADER
)

// Node The Node's state
type Node struct {
	id          commons.NodeID
	currentTerm commons.Term
	votedFor    commons.NodeID
	logs        []*LogEntry
	addr        commons.Addr
	nodesAddrs  []commons.Addr
	// volatile state
	lastCommited commons.Index
	lastApplied  commons.Index
	// ticker
	rw              sync.Mutex // holding tickers
	ticker          time.Ticker
	heartbeatTicker time.Ticker
	electionTimeout time.Duration
	nodeState       NodeState
	// For testing
	allowedToLog map[int32]struct{}
}

type server struct {
	pb.UnimplementedRaftServiceServer
	nd *Node
}

func (s *server) RequestVote(_ context.Context, in *pb.VoteRequest) (*pb.VoteResponse, error) {
	nd := s.nd
	if nd == nil {
		return nil, fmt.Errorf("Node not found")
	}
	//nd.log("Got a request from node %v", in.CandidateId)
	vote := nd.checkVote(in)
	//nd.log("Processed from node %v", in.CandidateId)
	return &pb.VoteResponse{
		CurrentTerm: int32(nd.currentTerm),
		VoteGranted: vote,
	}, nil
}

func (n *Node) swapAndCleanTicker(t1 *time.Ticker) {
	t2 := n.ticker
	n.ticker = *t1
	t2.Stop()
	<-t2.C // drain the channel
}

func (s *server) AppendEntry(_ context.Context, in *pb.AppendEntryRequest) (*pb.AppendEntryResponse, error) {
	nd := s.nd
	nd.log("Master acknowledged: %v", in.LeaderId)

	// reset timer
	//wg := sync.WaitGroup{}
	//wg.Add(1)
	//go func() {
	//	defer wg.Done()
	//	nd.ticker.Stop()
	//}()
	//wg.Wait()
	//nd.ticker.Stop()

	if commons.NodeID(in.LeaderId) != nd.id {
		nd.log("I conform to %d", in.LeaderId)
		nd.nodeState = FOLLOWER
	}
	// TODO: check if is heartbeat, respond differently

	//nd.swapAndCleanTicker(time.NewTicker(nd.electionTimeout))
	nd.ticker = *time.NewTicker(nd.electionTimeout)

	return &pb.AppendEntryResponse{
		Term:    int32(nd.currentTerm),
		Success: false,
	}, nil
}

func (n *Node) checkVote(vr *pb.VoteRequest) bool {
	if n.currentTerm < commons.Term(vr.Term) {
		// if node votedFor is nil or candidate id, proceed
		cid := commons.NodeID(vr.CandidateId)

		if n.votedFor == 0 || n.votedFor == cid {
			// check if log is up to date
			if n.lastCommited <= commons.Index(vr.LastLogIndex) {
				n.votedFor = cid
				log.Printf("%d Voted for %d", n.id, cid)
				return true
			}
		}
	}
	return false
}

func NewNode(id commons.NodeID, actionTimeout time.Duration, addr string, _nodeAddrs []string, ch chan os.Signal) *Node {
	a, err := url.Parse(addr)
	if err != nil {
		log.Fatalf("Cannot parse addr %v", err)
	}
	nodeAddrs := make([]commons.Addr, 0)
	for _, na := range _nodeAddrs {
		//if na == addr {
		//	continue
		//}
		nodeAddrs = append(nodeAddrs, commons.Addr(na))
	}

	allowedToLog := map[int32]struct{}{
		//0: {}, // only leader
	}

	nd := &Node{
		id:              id,
		currentTerm:     0,
		votedFor:        0,
		logs:            nil,
		addr:            commons.Addr(a.String()),
		nodesAddrs:      nodeAddrs,
		lastCommited:    0,
		lastApplied:     0,
		nodeState:       FOLLOWER,
		rw:              sync.Mutex{},
		electionTimeout: actionTimeout,
		ticker:          *time.NewTicker(actionTimeout),
		heartbeatTicker: *time.NewTicker(configs.HEARTBEAT_TIMEOUT),
		// testing
		allowedToLog: allowedToLog,
	}

	// start server and operations
	go nd.startNode(ch)
	go nd.operate(ch)

	return nd
}

func (n *Node) log(fmtLog string, args ...interface{}) {
	isLeader := n.nodeState == LEADER
	if _, ok := n.allowedToLog[int32(n.id)]; !ok && len(n.allowedToLog) > 0 {
		// !(a & b) == !a + !b
		if _, ok := n.allowedToLog[0]; !ok || !isLeader {
			return
		}
	}
	prefix := fmt.Sprintf("Node %d: ", n.id)
	if isLeader {
		prefix = fmt.Sprintf("(L)>%v", prefix)
	}
	msg := fmt.Sprintf(fmtLog, args...)
	log.Printf("%s%s", prefix, msg)
}

// manage and spin up server
func (n *Node) startNode(quit chan os.Signal) {
	// start the grpc server
	// listen for tcp
	lis, err := net.Listen("tcp", n.addr.GetHost())
	if err != nil {
		n.log("Node failed to listen, %v", err)
	}
	s := grpc.NewServer()
	defer s.Stop()

	n.log("Listener at %v", lis.Addr().Network())

	pb.RegisterRaftServiceServer(s, &server{nd: n})
	n.log("GRPC Server listening at %v", lis.Addr())
	// run server
	if err := s.Serve(lis); err != nil {
		log.Fatalf("Can't serve listener, %v", err)
	}
	// delay till signal
	signal := <-quit
	n.log("Closing grpc server: %v", signal)
}

// start operations
func (n *Node) operate(quit chan os.Signal) {
	// waits every election count which can be reset
	// constantly run
	log.Printf("Running node: %d", n.id)
	for {
		select {
		case <-n.ticker.C:
			// Declare candidateship
			n.handleElection()
		case <-n.heartbeatTicker.C:
			if n.nodeState == LEADER {
				n.handleAppendEntries(nil)
			}
		case sig := <-quit:
			n.log("Node got call %s", sig)
			return // track signal later
		}
	}
}

// using routines to apply to all nodes
func (n *Node) applyToNodes(fn func(string)) {
	n.log("------------Operation------")
	wg := sync.WaitGroup{}
	for _, addr := range n.nodesAddrs {
		wg.Add(1)
		go func(addr commons.Addr) {
			defer wg.Done()
			fn(addr.GetHost())
		}(addr)
	}
	wg.Wait()
}

func (n *Node) sendToMaster() {}

func (n *Node) handleElection() {
	n.nodeState = CANDIDATE
	n.log("+++++++++++++++++++ Election period from %d", n.id)
	var count int32
	// Perform metric on fn
	t1 := time.Now()
	n.applyToNodes(func(s string) {
		// hold access to update count
		n.rw.Lock()
		defer n.rw.Unlock()
		d := n.declareCandidate(s)
		if d.VoteGranted {
			// using atomic increment for count
			atomic.AddInt32(&count, 1)
		}
		n.log("Count on %v: %v", n.currentTerm, count)
	})
	n.log("___--------_______--------_______-------_______-----____---> Election time: %v", time.Since(t1))
	count = atomic.LoadInt32(&count)
	if commons.HasPriorityVotes(len(n.nodesAddrs), int(count)) {
		log.Printf("Node %d: Vote managed, count: %d", n.id, count)
		// declare leadership by sending heartbeat
		n.log("-----------> I am leader")
		n.nodeState = LEADER
		n.handleAppendEntries(nil)
	}
}

func (n *Node) handleAppendEntries(entries []string) {
	if entries == nil {
		n.log("Heartbeat")
	}
	n.applyToNodes(func(s string) {
		n.sendAppendEntries(s, entries)
	})
}

func (n *Node) declareCandidate(addr string) *pb.VoteResponse {

	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	n.log("Declaring candidateship to %v", addr)
	if err != nil {
		log.Fatalf("Can't send connection")
	}
	defer conn.Close()

	c := pb.NewRaftServiceClient(conn)

	// make context for reqs
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	// contact addr
	lastLogIndex := len(n.logs)

	// update currentTerm and request vote
	n.currentTerm += 1

	d, err := c.RequestVote(ctx, &pb.VoteRequest{
		Term:         int32(n.currentTerm),
		CandidateId:  int32(n.id),
		LastLogTerm:  0, // int32((*n.logs[lastLogIndex]).term)
		LastLogIndex: int32(lastLogIndex),
	})
	if err != nil {
		n.log("Error declaring node: as candidate to %s, %v", addr, err)
		return d // FIXME: add err management
	}

	n.log("Response, %d, voted: %v", d.CurrentTerm, d.VoteGranted)

	return d
}

func (n *Node) sendAppendEntries(addr string, entries []string) *pb.AppendEntryResponse {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Can't send entry connection!")
	}
	defer conn.Close()

	// make context for req
	c := pb.NewRaftServiceClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// send rpc request
	lastLogIndex := len(n.logs) - 1
	d, err := c.AppendEntry(ctx, &pb.AppendEntryRequest{
		Term:         int32(n.currentTerm),
		LeaderId:     int32(n.id), // is it my id since I'm leader ???
		PrevLogIndex: int32(lastLogIndex),
		PrevLogTerm:  int32(n.lastCommited),
		Entries:      entries,
		LeaderCommit: 0,
	})

	if err != nil {
		n.log("Error sending entry to %v, %v", addr, err)
		return d // FIXME: add err management
	}
	n.log("Sent to %v; resp: %v", addr, d.Success)

	return d
}
