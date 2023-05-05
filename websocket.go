package main

import (
	"bytes"
	"compress/zlib"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

type MinimalMessage struct {
	Content   string `json:"content"`
	ChannelID string `json:"id"`
	Author    Author `json:"author"`
}

type RedeemResponse struct {
	code int
}

type Author struct {
	Id       string `json:"id"`
	Username string `json:"username"`
	Discrim  string `json:"discriminator"`
}

type IdentifyProperties struct {
	OS              string `json:"$os"`
	Browser         string `json:"$browser"`
	UserAgent       string `json:"browser_user_agent"`
	Device          string `json:"$device"`
	Referer         string `json:"$referer"`
	ReferringDomain string `json:"$referring_domain"`
}

type GatewayStatusUpdate struct {
	Since  int    `json:"since"`
	Status string `json:"status"`
	AFK    bool   `json:"afk"`
}

type Identify struct {
	Token      *string             `json:"token"`
	Properties IdentifyProperties  `json:"properties"`
	Compress   bool                `json:"compress"`
	Presence   GatewayStatusUpdate `json:"presence,omitempty"`
	Intents    int                 `json:"intents"`
}

type DiscordGatewayClient struct {
	token  string
	ws     *websocket.Conn
	dialer *websocket.Dialer
	Identify
	heartbeatt       int
	LastHeartbeatAck time.Time
	sessionID        string
	sequence         int
	URL              string
	closed           chan struct{}
	Guilds           int
}

type Event struct {
	Operation int             `json:"op"`
	Sequence  int64           `json:"s"`
	Type      string          `json:"t"`
	RawData   json.RawMessage `json:"d"`
	// Struct contains one of the other types in this file.
	Struct interface{} `json:"-"`
}

type helloOp struct {
	HeartbeatInterval time.Duration `json:"heartbeat_interval"`
}

type identifyOp struct {
	Op   int      `json:"op"`
	Data Identify `json:"d"`
}

type heartbeatOp struct {
	Op   int    `json:"op"`
	Data string `json:"d"`
}

type readyOp struct {
	ResumeGatewayUrl string `json:"resume_gateway_url"`
	SessionId        string `json:"session_id"`
}

func NewDiscordClient(token string) (dg *DiscordGatewayClient) {
	dg = &DiscordGatewayClient{
		token: token,
		URL:   "wss://gateway.discord.gg/?encoding=json&v=9",
	}

	dg.Identify.Token = &dg.token
	dg.Identify.Compress = true
	dg.Identify.Presence.Status = "online"
	dg.Identify.Properties.Browser = "Chrome"
	dg.Identify.Properties.UserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/108.0.0.0 Safari/537.36"
	dg.Identify.Properties.OS = "Mac OS X"
	dg.Identify.Intents = 1<<9 | 1<<12 | 1<<15 //guild msgs, dm msgs, msg content
	return
}

func (dg *DiscordGatewayClient) Run() (err error) {
	defer func() {
		if err != nil && dg.ws != nil {
			dg.close()
		}
	}()

	header := http.Header{}
	header.Add("accept-encoding", "zlib")

	dg.ws, _, err = websocket.DefaultDialer.Dial(dg.URL, header)
	if err != nil {
		return err
	}

	mt, m, err := dg.ws.ReadMessage()
	if err != nil {
		return fmt.Errorf("failed to read hello packet %w;", err)
	}
	e, err := dg.HandleMessage(mt, m)
	if err != nil {
		return fmt.Errorf("failed to decode hello packet %w;", err)
	}

	if e.Operation != 10 {
		return fmt.Errorf("expecting hello packet, received: %d %w;", e.Operation, err)
	}

	dg.LastHeartbeatAck = time.Now().UTC()

	var h helloOp
	if err = json.Unmarshal(e.RawData, &h); err != nil {
		return fmt.Errorf("failed to unmarshal hello packet %w;", err)

	}

	op := identifyOp{2, dg.Identify}
	err = dg.ws.WriteJSON(op)
	if err != nil {
		return fmt.Errorf("Failed to send identify packet %w;", err)
	}

	mt, m, err = dg.ws.ReadMessage()
	if err != nil {
		return fmt.Errorf("Failed to read ready packet %w;", err)
	}

	e, err = dg.HandleMessage(mt, m)
	if err != nil {
		return fmt.Errorf("Failed to decode ready packet %w;", err)
	}

	if e.Type == "READY" {
		var r readyOp
		if err = json.Unmarshal(e.RawData, &r); err != nil {
			return fmt.Errorf("Failed to unmarshal ready packet %w;", err)
		}
		dg.sessionID = r.SessionId
		dg.URL = r.ResumeGatewayUrl
	}

	e, err = dg.HandleMessage(mt, m)
	if err != nil {
		return fmt.Errorf("Failed to decode ready packet %w;", err)
	}

	var r map[string]interface{}
	if err = json.Unmarshal(e.RawData, &r); err != nil {
		return fmt.Errorf("Failed to unmarshal ready packet %w;", err)
	}
	dg.Guilds = len(r["guilds"].([]interface{}))

	go dg.heartbeat(dg.ws, h.HeartbeatInterval)
	go dg.listen(dg.ws)

	return nil
}

func (dg *DiscordGatewayClient) HandleMessage(mt int, m []byte) (*Event, error) {
	var err error
	var reader io.Reader
	reader = bytes.NewBuffer(m)

	// If this is a compressed message, uncompress it.
	if mt == websocket.BinaryMessage {
		z, err2 := zlib.NewReader(reader)
		if err2 != nil {
			return nil, fmt.Errorf("Failed to decode zlib packet %w;", err)
		}

		defer func() error {
			err3 := z.Close()
			if err3 != nil {
				return fmt.Errorf("Failed to decode zlib packet %w;", err)
			}
			return nil
		}()

		reader = z
	}

	var e *Event
	decoder := json.NewDecoder(reader)
	if err = decoder.Decode(&e); err != nil {
		return e, fmt.Errorf("Failed to decode websocket json %w;", err)
	}

	dg.sequence = int(e.Sequence)

	switch e.Operation {
	case 7: //discord says reconnect
		dg.reconnect()
	case 4000: //unknown error, reconnect
		dg.reconnect()
	case 4001: //invalid opcode, reconnect
		dg.reconnect()
	case 4002: //invalid payload, reconnect
		dg.reconnect()
	case 4004: //token invalid, error and exit
		return nil, errors.New("Invalid Token")
	case 4007: //invalid seq reconnect
		dg.reconnect()
	case 4008: //rate limit, reconnect
		dg.reconnect()
	case 4009: //session expired reconnect
		dg.reconnect()
	case 4012: //invalid api version, exit
		return nil, errors.New("Invalid API version")
	case 4013: //invalid intents exit
		return nil, errors.New("Invalid intents")
	case 4014: //diasallowed intents exit
		return nil, errors.New("Disallowed intents")
	}

	if e.Type == "MESSAGE_CREATE" || e.Type == "MESSAGE_UPDATE" {
		var msg *MinimalMessage

		if err := json.Unmarshal(e.RawData, &msg); err != nil {
			return nil, fmt.Errorf("Failed to decode discord message %w;", err)
		}
		go dg.HandleDiscordMessage(msg)
	}

	return e, nil
}

func (dg *DiscordGatewayClient) reconnect() {
	dg.Run()
}

func (dg *DiscordGatewayClient) heartbeat(wsConn *websocket.Conn, heartbeatIntervalMsec time.Duration) (err error) {

	if wsConn == nil {
		return errors.New("heartbeat ws is nil")
	}

	ticker := time.NewTicker(heartbeatIntervalMsec * time.Millisecond)
	defer ticker.Stop()

	for {
		err = wsConn.WriteJSON(heartbeatOp{1, "null"})
		if err != nil {
			WarningLogger.Println("Failed to send heartbeat ", err)
			return
		}
		select {
		case <-ticker.C:
			// continue loop and send heartbeat

		}
	}
}

func (dg *DiscordGatewayClient) listen(wsConn *websocket.Conn) {
	for {

		mt, m, err := wsConn.ReadMessage()

		if err != nil {
			dg.close() //dont need to do this on the heartbeat as it will just send it twice
			return
		}

		select {

		default:
			e, err := dg.HandleMessage(mt, m)
			if err != nil {
				_ = e
				return
			}
		}
	}
}

func (dg *DiscordGatewayClient) close() {
	dg.ws.Close()
	<-dg.closed
}

func (dg *DiscordGatewayClient) redeemNitro(code string, channelId string) (*http.Response, error) {

	var jsonData = []byte(fmt.Sprintf(`{
		"channel_id": "%s",
		"gateway_checkout_context": null
	  }`, channelId))

	client := &http.Client{}
	req, err := http.NewRequest("POST", fmt.Sprintf("https://discordapp.com/api/v9/entitlements/gift-codes/%s/redeem", code), bytes.NewBuffer(jsonData))

	if err != nil {
		ErrorLogger.Println("Failed to create redeem request ", err)
		return nil, err
	}
	req.Header.Add("Authorization", cfg.Token)
	req.Header.Add("content-type", "application/json")
	req.Header.Add("payment_source_id", "null")

	resp, err := client.Do(req)
	if err != nil {
		b, err2 := httputil.DumpResponse(resp, true)
		if err2 != nil {
			ErrorLogger.Println("Failed to dump redeem response ", err2)
			return nil, err
		}
		ErrorLogger.Println("Failed to send redeem request ", string(b))

		return nil, err
	}

	start, err := SnowflakeTimestamp(channelId)
	if err != nil {
		WarningLogger.Println("Failed to convert snowflake to timestamp: ", channelId)
	}

	end := time.Now().Sub(start).Abs()
	fmt.Printf("Redeem took: %s\n", end)

	return resp, nil
}

func (dg *DiscordGatewayClient) HandleDiscordMessage(m *MinimalMessage) {
	if strings.Contains(m.Content, "discord.gift") {
		split := strings.Split(m.Content, "discord.gift/")
		fmt.Println(len(split) - 1)
		fmt.Println("found discord.gift message")
		resp, err := dg.redeemNitro(split[len(split)-1], m.ChannelID)
		if err != nil {
			return
		}
		if resp.StatusCode == 200 {
			SuccessLogger.Println("Successfully redeemed nitro: ", split[len(split)-1])
		}
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return
		}
		bodyString := string(bodyBytes)
		fmt.Println(bodyString)

	}
}

func SnowflakeTimestamp(ID string) (t time.Time, err error) {
	i, err := strconv.ParseInt(ID, 10, 64)
	if err != nil {
		return
	}
	timestamp := (i >> 22) + 1420070400000
	t = time.Unix(0, timestamp*1000000)
	return
}
