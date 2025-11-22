package main

import (
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sandertv/go-raknet"
)

type Config struct {
	Target        string
	MaxConnections int
	Duration      time.Duration
}

type ConnectionManager struct {
	config       *Config
	connections  sync.Map
	activeConns  int32
	totalConns   int32
	failedConns  int32
	stopChan     chan struct{}
	wg           sync.WaitGroup
}

func NewConnectionManager(config *Config) *ConnectionManager {
	return &ConnectionManager{
		config:   config,
		stopChan: make(chan struct{}),
	}
}

func (cm *ConnectionManager) Start() {
	// Iniciar workers
	workers := 50
	if cm.config.MaxConnections < 1000 {
		workers = 20
	}

	for i := 0; i < workers; i++ {
		cm.wg.Add(1)
		go cm.connectionWorker(i)
	}

	// Timer de parada automática
	if cm.config.Duration > 0 {
		cm.wg.Add(1)
		go cm.durationTimer()
	}

	cm.wg.Wait()
}

func (cm *ConnectionManager) Stop() {
	close(cm.stopChan)
	cm.wg.Wait()
	
	// Cerrar todas las conexiones
	cm.connections.Range(func(key, value interface{}) bool {
		if conn, ok := value.(*raknet.Conn); ok {
			conn.Close()
		}
		return true
	})
}

func (cm *ConnectionManager) durationTimer() {
	defer cm.wg.Done()
	
	select {
	case <-time.After(cm.config.Duration):
		cm.Stop()
	case <-cm.stopChan:
		return
	}
}

func (cm *ConnectionManager) connectionWorker(workerID int) {
	defer cm.wg.Done()

	for {
		select {
		case <-cm.stopChan:
			return
		default:
			if atomic.LoadInt32(&cm.activeConns) >= int32(cm.config.MaxConnections) {
				time.Sleep(100 * time.Millisecond)
				continue
			}

			err := cm.createConnection()
			if err != nil {
				atomic.AddInt32(&cm.failedConns, 1)
				time.Sleep(50 * time.Millisecond)
			}
		}
	}
}

func (cm *ConnectionManager) createConnection() error {
	conn, err := raknet.Dial(cm.config.Target)
	if err != nil {
		return err
	}

	connID := atomic.AddInt32(&cm.totalConns, 1)
	atomic.AddInt32(&cm.activeConns, 1)

	cm.connections.Store(connID, conn)

	cm.wg.Add(1)
	go cm.handleConnection(connID, conn)

	return nil
}

func (cm *ConnectionManager) handleConnection(connID int32, conn *raknet.Conn) {
	defer cm.wg.Done()
	defer atomic.AddInt32(&cm.activeConns, -1)
	defer cm.connections.Delete(connID)
	defer conn.Close()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-cm.stopChan:
			return
		case <-ticker.C:
			_, err := conn.ReadPacket()
			if err != nil {
				return
			}
		}
	}
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

	manager := NewConnectionManager(config)
	manager.Start()
}
