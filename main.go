package main

import (
	"fmt"
	"log"
	"math/rand"
	"sort"
	"sync"
	"time"
)

// Command: request/release

const Interval = 1 * time.Millisecond
const RandRange = 10

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
	me         int
	clock      int
	queue      []Message
	recv       chan Message
	send       chan Message
	n          int
	mu         sync.Mutex
	lastClock  []int
	_channels  *[][]chan Message
	haveSource bool
}

func NewProcess(i int, n int, _channels *[][]chan Message) *Process {
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
		false,
	}
}

// blocking?

func (p *Process) granted() bool {
	for !p._granted() {
		time.Sleep(Interval * time.Duration(rand.Intn(RandRange)))
	}
	p.haveSource = true
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
	p.haveSource = false
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
				for msg := range (*p._channels)[i][p.me] {
					log.Printf("%v.%d recv: %v\n", S(p.me), p.clock, msg)
					p.recv <- msg
					time.Sleep(Interval * time.Duration(rand.Intn(RandRange)))
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
			(*p._channels)[p.me][msg.Dst] <- msg
			time.Sleep(Interval * time.Duration(rand.Intn(RandRange)))
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
		for {
			if p.haveSource == false {
				p.request()
			}
			p.granted()
			// enter CS
			log.Printf("%v.%d using\n", S(p.me), p.clock)

			p.release()
			time.Sleep(Interval * time.Duration(rand.Intn(RandRange)))
		}
	}()
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	n := 5

	channels := make([][]chan Message, n)
	for i := range channels {
		channels[i] = make([]chan Message, n)
		for j := range channels[i] {
			if i == j {
				continue
			}
			channels[i][j] = make(chan Message, 1)
		}
	}

	initp := 0
	initc := -1
	msg0 := Message{"Request", initp, initp, initc}

	processQueue := make([]*Process, n)
	for i, p := range processQueue {
		p = NewProcess(i, n, &channels)
		if i == 0 {
			p.haveSource = true
		}
		p.queue = append(p.queue, msg0)
		go p.start()
	}
	ch := make(chan int)
	<-ch
}
