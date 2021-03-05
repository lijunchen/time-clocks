package main

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

// Command: acquire/release

type Message struct {
	Type  string // Request, Release, Ack
	Src   int
	Dst   int
	Clock int
}

type TotalOrdering []Message

func (a TotalOrdering) Len() int {
	return len(a)
}

func (a TotalOrdering) Less(i, j int) bool {
	if a[i].Clock != a[j].Clock {
		return a[i].Clock < a[j].Clock
	}
	return a[i].Src < a[j].Src
}

func (a TotalOrdering) Swap(i, j int) {
	a[i], a[j] = a[j], a[i]
}

func removeById(messages []Message, id int) []Message {
	j := 0
	for _, message := range messages {
		if message.Src != id {
			messages[j] = message
			j++
		}
	}
	return messages[:j]
}

type Process struct {
	id        int
	clock     int
	queue     []Message
	recv      chan Message
	send      chan Message
	n         int
	mu        sync.Mutex
	lastClock []int
	_channels *[10][10]chan Message
}

func (p *Process) check() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	sort.Sort(TotalOrdering(p.queue))
	if len(p.queue) == 0 {
		panic("queue is empty")
	}
	cond1 := p.queue[0].Src == p.id
	if !cond1 {
		return false
	}
	conds := make([]bool, p.n)
	conds[p.id] = true
	for i, c := range p.lastClock {
		if c > p.queue[0].Clock {
			conds[i] = true
		}
	}
	for _, b := range conds {
		if !b {
			return false
		}
	}
	fmt.Println(conds)
	return true
}

func (p *Process) start() {
	// receiver
	// ? -> id
	fmt.Printf("P%d start\n", p.id)
	go func() {
		for i := 0; i < p.n; i++ {
			if i == p.id {
				continue
			}
			go func(i int) {
				for msg := range p._channels[i][p.id] {
					p.recv <- msg
					time.Sleep(1 * time.Millisecond)
				}
			}(i)
		}
	}()

	// sender
	// id -> ?
	go func() {
		for msg := range p.send {
			p._channels[p.id][msg.Dst] <- msg
			time.Sleep(1 * time.Millisecond)
		}
	}()

	go func() {
		for msg := range p.recv {
			p.mu.Lock()
			if msg.Clock > p.clock {
				p.clock = msg.Clock
			}
			if msg.Type == "Request" {
				p.queue = append(p.queue, msg)
				ack := Message{"Ack", 0, msg.Src, p.clock}
				p.send <- ack
			} else if msg.Type == "Release" {
				p.queue = removeById(p.queue, msg.Src)
			} else if msg.Type == "Ack" {
			} else {
				panic("unknown message type")
			}
			p.clock++
			p.lastClock[msg.Src] = msg.Clock
			p.mu.Unlock()
		}
	}()

	go func() {
		for {
			// acquire
			fmt.Printf("P%d acquire,\t clock: %d\n", p.id, p.clock)
			p.mu.Lock()
			for i := 0; i < p.n; i++ {
				acquireMessage := Message{"Request", p.id, i, p.clock}
				if i == p.id {
					p.queue = append(p.queue, acquireMessage)
					continue
				}
				p.send <- acquireMessage
			}
			p.clock++
			p.mu.Unlock()

			// use
			for {
				// check
				if p.check() {
					fmt.Printf("P%d using,\t clock: %d\n", p.id, p.clock)
					break
				} else {
					time.Sleep(1 * time.Millisecond)
				}
			}
			p.mu.Lock()
			p.clock++
			p.mu.Unlock()

			// release
			fmt.Printf("P%d release,\t clock: %d\n", p.id, p.clock)
			p.mu.Lock()
			removeById(p.queue, p.id)
			for i := 0; i < p.n; i++ {
				if i == p.id {
					continue
				}
				acquireMessage := Message{"Release", p.id, i, p.clock}
				p.send <- acquireMessage
			}
			p.clock++
			p.mu.Unlock()
			time.Sleep(1 * time.Millisecond)
		}
	}()
}

func main() {
	channels := [10][10]chan Message{}
	for i := 0; i < 10; i++ {
		for j := 0; j < 10; j++ {
			if i == j {
				continue
			}
			channels[i][j] = make(chan Message, 10)
		}
	}
	n := 3
	p0 := Process{
		0,
		0,
		[]Message{},
		make(chan Message),
		make(chan Message),
		n,
		sync.Mutex{},
		make([]int, n),
		&channels,
	}
	initp := 0
	initc := -1
	msg0 := Message{"Request", initp, initp, initc}
	p0.queue = append(p0.queue, msg0)
	fmt.Println(p0)

	p1 := Process{
		1,
		0,
		[]Message{},
		make(chan Message),
		make(chan Message),
		n,
		sync.Mutex{},
		make([]int, n),
		&channels,
	}
	p1.queue = append(p1.queue, msg0)

	p2 := Process{
		2,
		0,
		[]Message{},
		make(chan Message),
		make(chan Message),
		n,
		sync.Mutex{},
		make([]int, n),
		&channels,
	}
	p2.queue = append(p2.queue, msg0)

	go p0.start()
	go p1.start()
	go p2.start()

	ch := make(chan int)
	<-ch
}
