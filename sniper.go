package main

import "fmt"

type ClientDisconnectMsg struct {
	client *DiscordGatewayClient
}

type Sniper struct {
	tokens     []string
	connected  int
	guildCount int

	clients []*DiscordGatewayClient
	msgch   chan any
}

func NewSniper(tokens []string) (*Sniper, error) {
	return &Sniper{
		tokens: tokens,
	}, nil
}

func (s *Sniper) Start() {
	s.msgch = s.loop()
	s.startClients()
}

func (s *Sniper) loop() chan any {
	msgch := make(chan any, 1)
	go func() {
		for msg := range msgch {
			ClearTerminal()
			fmt.Printf("Connected: %d\nGuild count: %d", s.connected, s.guildCount)
			switch m := msg.(type) {
			case *DiscordGatewayClient:
				s.addClient(m)
			case *ClientDisconnectMsg:
				s.removeClient(m.client)
			}
		}
	}()
	return msgch
}

func (s *Sniper) addClient(client *DiscordGatewayClient) {
	s.clients = append(s.clients, client)
	s.connected += 1
	s.guildCount += client.Guilds
}

func (s *Sniper) removeClient(client *DiscordGatewayClient) {
	s.connected -= 1
	s.guildCount -= client.Guilds
}

func (s *Sniper) startClients() {
	go func() {
		for _, t := range s.tokens {
			// time.Sleep(time.Duration(s.cfg.Cooldown) * time.Second) //sleep to prevent too many heartbeats firing, causing rate limiting.. just a theory
			go func() {
				d := NewDiscordClient(t)
				err := d.Run()
				if err != nil {
					WarningLogger.Println("Failed to run token ", err)
					return
				}
				s.msgch <- d
				<-d.closed
				s.msgch <- &ClientDisconnectMsg{
					client: d,
				}
			}()
		}
	}()
}
