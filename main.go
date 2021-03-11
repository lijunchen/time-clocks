package main

import (
	"fmt"
	"log"
	"sort"
	"sync"
	"time"
)

// Command: request/release

const Interval = 1 * time.Millisecond

type Message struct {
	Type  string // Request, Release, Ack
	Src   int
	Dst   int
	Clock int
}

func S(i int) string {
	return string(65 + i)
}

func (m Message) String() string {
	return fmt.Sprintf("%v--[%v:%v]-->%v", S(m.Src), m.Type, m.Clock, S(m.Dst))
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

func removeBySrc(messages []Message, src int) []Message {
	j := 0
	for _, message := range messages {
		if message.Src != src {
			messages[j] = message
			j++
		}
	}
	return messages[:j]
}

type Process struct {
	me        int
	clock     int
	queue     []Message
	recv      chan Message
	send      chan Message
	n         int
	mu        sync.Mutex
	lastClock []int
	_channels *[10][10]chan Message
}

func NewProcess(i int, n int, _channels *[10][10]chan Message) *Process {
	return &Process{
		i,
		0,
		[]Message{},
		make(chan Message),
		make(chan Message),
		n,
		sync.Mutex{},
		make([]int, n),
		_channels,
	}
}

// blocking?

func (p *Process) granted() bool {
	for !p._granted() {
		time.Sleep(Interval)
	}
	p.mu.Lock()
	p.clock++
	p.mu.Unlock()
	return true
}

func (p *Process) _granted() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	sort.Sort(TotalOrdering(p.queue))
	if len(p.queue) == 0 {
		panic("queue is empty")
	}
	cond1 := p.queue[0].Src == p.me
	if !cond1 {
		log.Printf("%v.%d cond1 check failed\n", S(p.me), p.clock)
		return false
	}
	conds := make([]bool, p.n)
	conds[p.me] = true
	for i, c := range p.lastClock {
		if c > p.queue[0].Clock {
			conds[i] = true
		}
	}
	for _, b := range conds {
		if !b {
			log.Printf("%v.%d conds check failed\n", S(p.me), p.clock)
			return false
		}
	}
	return true
}

func (p *Process) request() {
	log.Printf("%v.%d request\n", S(p.me), p.clock)
	p.mu.Lock()
	for i := 0; i < p.n; i++ {
		requestMessage := Message{"Request", p.me, i, p.clock}
		if i == p.me {
			p.queue = append(p.queue, requestMessage)
			continue
		}
		p.send <- requestMessage
	}
	p.clock++
	p.mu.Unlock()
}

func (p *Process) release() {
	log.Printf("%v.%d release\n", S(p.me), p.clock)
	p.mu.Lock()
	p.queue = removeBySrc(p.queue, p.me)
	for i := 0; i < p.n; i++ {
		if i == p.me {
			continue
		}
		requestMessage := Message{"Release", p.me, i, p.clock}
		p.send <- requestMessage
	}
	p.clock++
	p.mu.Unlock()
}

func (p *Process) start() {
	// the receiver channel
	// ? -> me
	log.Printf("%v start\n", S(p.me))
	go func() {
		for i := 0; i < p.n; i++ {
			if i == p.me {
				continue
			}
			go func(i int) {
				// i -> me
				for msg := range p._channels[i][p.me] {
					log.Printf("%v.%d recv: %v\n", S(p.me), p.clock, msg)
					p.recv <- msg
					time.Sleep(Interval)
				}
			}(i)
		}
	}()

	// the sender channel
	// me -> ?
	go func() {
		for msg := range p.send {
			// me -> msg.Dst
			log.Printf("%v.%d send: %v\n", S(p.me), p.clock, msg)
			p._channels[p.me][msg.Dst] <- msg
			time.Sleep(Interval)
		}
	}()

	// handle message from the reveiver channel
	go func() {
		for msg := range p.recv {
			p.mu.Lock()
			p.lastClock[msg.Src] = msg.Clock
			if msg.Clock > p.clock {
				p.clock = msg.Clock + 1
			}
			if msg.Type == "Request" {
				p.queue = append(p.queue, msg)
				ack := Message{"Ack", p.me, msg.Src, p.clock}
				p.send <- ack
			} else if msg.Type == "Release" {
				p.queue = removeBySrc(p.queue, msg.Src)
				ack := Message{"Ack", p.me, msg.Src, p.clock}
				p.send <- ack
			} else if msg.Type == "Ack" {
				// empty
			} else {
				panic("unknown message type")
			}
			p.clock++
			p.mu.Unlock()
		}
	}()

	go func() {
		first := true
		for {
			if first {
				first = false
				if p.me != 0 {
					p.request()
				}
			} else {
				p.request()
			}

			p.granted()
			// enter CS
			log.Printf("%v.%d using\n", S(p.me), p.clock)
			time.Sleep(1 * Interval)

			p.release()
			time.Sleep(Interval)
		}
	}()
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	channels := [10][10]chan Message{}
	for i := 0; i < 10; i++ {
		for j := 0; j < 10; j++ {
			if i == j {
				continue
			}
			channels[i][j] = make(chan Message, 1)
		}
	}
	n := 3

	initp := 0
	initc := -1
	msg0 := Message{"Request", initp, initp, initc}

	p0 := NewProcess(0, n, &channels)
	p0.queue = append(p0.queue, msg0)

	p1 := NewProcess(1, n, &channels)
	p1.queue = append(p1.queue, msg0)

	p2 := NewProcess(2, n, &channels)
	p2.queue = append(p2.queue, msg0)

	go p0.start()
	go p1.start()
	go p2.start()
	go func() {
		for true {
			time.Sleep(Interval)
		}
	}()

	ch := make(chan int)
	<-ch
}
