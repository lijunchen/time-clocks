package main

import (
	"log"
	"net"
	"net/rpc"
	"sync"
)

// Command: acquire/release

type Message struct {
	Clock int
	Id    int
}

type Process struct {
	Id           int
	RequestQueue []Message
	Clock        int

	peers []*rpc.Client
	x     int
	lk    sync.Mutex
}

func (p *Process) request() {

}

func (p *Process) release() {

}

func (p *Process) Receive(args *Args, reply *Reply) error {
	return nil
}

type Args struct {
	Type  string // Request, Release, Ack
	Id    int
	Clock int
}

type Reply struct {
	Ok bool
}

func (p *Process) start() {
	for i := 0; i < 3; i++ {
		log.Printf("Process %d, ++\n", p.Id)
		p.request()
		// some mutual exclusion operations
		p.x++ // dummy op
		p.release()
	}
}

func main() {
	// init
	addrs := []string{
		"127.0.0.1:10000",
		"127.0.0.1:10001",
		"127.0.0.1:10002",
	}

	processes := []*Process{}
	for i := range addrs {
		process := Process{i, []Message{}, -1, nil, 0, sync.Mutex{}}
		processes = append(processes, &process)
	}

	// init rcp server
	for i, addr := range addrs {
		handler := rpc.NewServer()
		handler.Register(processes[i])
		listener, err := net.Listen("tcp", addr)
		if err != nil {
			log.Fatalf("listen %v error", addr)
		}
		log.Printf("Processes %d listening on %v\n", i, listener.Addr())

		go func(i int) {
			for {
				conn, err := listener.Accept()
				if err != nil {
					log.Fatalf("Processes %d accept error", i)
				}
				log.Printf("Server %d accepted connection from %s\n", i, conn.RemoteAddr())
				go handler.ServeConn(conn)
			}
		}(i)
	}

	// init rpc client, peers
	for i, process := range processes {
		peers := []*rpc.Client{}
		for j, addr := range addrs {
			if i == j {
				peers = append(peers, nil)
				continue
			}
			peer, err := rpc.Dial("tcp", addr)
			if err != nil {
				log.Fatalf("Dial: %v error", addr)
			}
			peers = append(peers, peer)
		}
		process.peers = peers
	}

	var wg sync.WaitGroup
	wg.Add(len(addrs))
	for _, process := range processes {
		go func(process *Process) {
			defer wg.Done()
			process.start()
		}(process)
	}
	wg.Wait()
}
