package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
)

var (
	hostname, LLM, systemPrompt, keepAlive string
)

func request(url string, timeout int, header map[string]string, body []byte, Type string) ([]byte, int, error) {
	req, err := http.NewRequest(Type, url, strings.NewReader(string(body)))
	if err != nil {
		return nil, 0, fmt.Errorf("http.NewRequest:%v", err)
	}
	for key, value := range header {
		req.Header.Set(key, value)
	}
	client := &http.Client{
		Timeout: time.Duration(timeout) * time.Second,
	}
	data, err := client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("client.Do:%v", err)
	}
	defer data.Body.Close()
	databytes, err := io.ReadAll(data.Body)
	if err != nil {
		return nil, data.StatusCode, fmt.Errorf("io.ReadAll:%v", err)
	}
	return databytes, data.StatusCode, nil
}
func sendChat(url string, data map[string]any) (chatResponse string, err error) {
	defer func() {
		r := recover()
		if r != nil {
			err = fmt.Errorf("%v", r)
		}
	}()
	url += "api/chat"
	header := map[string]string{
		"Content-Type": "application/json",
	}
	dataJson, err := json.Marshal(data)
	if err != nil {
		err = fmt.Errorf("json make error:%v", err)
		return
	}
	responseJson, code, err := request(url, 300, header, dataJson, "POST")
	if err != nil {
		return
	} else if code != 200 {
		err = fmt.Errorf("http status code:%v, response:%v", code, string(responseJson))
		return
	}
	var response map[string]any
	err = json.Unmarshal(responseJson, &response)
	if err != nil {
		err = fmt.Errorf("json parsing error:%v,%v", err, responseJson)
		return
	}
	chatResponse = response["message"].(map[string]any)["content"].(string)
	return
}
func sendGenerate(url string, data map[string]any) (chatResponse string, err error) {
	url += "api/generate"
	header := map[string]string{
		"Content-Type": "application/json",
	}
	dataJson, err := json.Marshal(data)
	if err != nil {
		err = fmt.Errorf("json parsing error:%v", err)
		return
	}
	responseJson, code, err := request(url, 300, header, dataJson, "POST")
	if err != nil {
		return
	} else if code != 200 {
		err = fmt.Errorf("http status code:%v, response:%v", code, string(responseJson))
		return
	}
	var response map[string]any
	err = json.Unmarshal(responseJson, &response)
	if err != nil {
		err = fmt.Errorf("json parsing error:%v,%v", err, responseJson)
		return
	}
	chatResponse, ok := response["response"].(string)
	if !ok {
		err = fmt.Errorf("json parsing error: done 'key' not found,%v", responseJson)
	}
	return
}
func createPrompt(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Type != discordgo.MessageTypeDefault {
		return
	}
	if m.Author.ID == s.State.User.ID {
		return
	}
	if !strings.Contains(m.Content, fmt.Sprintf("<@%s>", s.State.User.ID)) {
		return
	}
	stop := false
	go func() {
		for !stop {
			err := s.ChannelTyping(m.ChannelID)
			if err != nil {
				fmt.Println("Error indicating typing:", err)
			}
			time.Sleep(time.Second * 9)
		}
	}()
	var send func(url string, data map[string]any) (chatResponse string, err error)
	var data map[string]any
	if false && len(m.Attachments) > 0 {
		var images []string
		for _, v := range m.Attachments {
			if v.ContentType != "image/jpeg" && v.ContentType != "image/png" {
				continue
			}
			image, status, err := request(v.URL, 60, nil, nil, "GET")
			if err != nil {
				fmt.Println("Error downloading images")
			} else if status != 200 {
				fmt.Printf("error downloading images, status: %d, response: %s\n", status, string(image))
			} else {
				images = append(images, base64.StdEncoding.EncodeToString(image))
			}
		}
		message := strings.ReplaceAll(m.Content, fmt.Sprintf("<@%s>", s.State.User.ID), "")
		if len(images) != 0 {
			send = sendGenerate
			data = map[string]any{
				"model":      "llava:13b",
				"keep_alive": "0m",
				"prompt":     message,
				"images":     images,
				"stream":     false,
			}
		} else {
			send = func(url string, data map[string]any) (chatResponse string, err error) {
				chatResponse = "invalid attachments"
				return
			}
		}
	} else {
		nickname := make(map[string]string)
		var messages []map[string]string
		chatHistory, err := s.ChannelMessages(m.ChannelID, 100, m.ID, "", "")
		if err == nil {
			for i := len(chatHistory) - 1; i >= 0; i-- {
				if chatHistory[i].Content == "" {
					continue
				}
				id := chatHistory[i].Author.ID
				if nickname[id] == "" {
					member, err := s.GuildMember(m.GuildID, id)
					if err != nil || member.Nick == "" {
						nickname[id] = chatHistory[i].Author.Username
					} else {
						nickname[id] = member.Nick
					}
				}
				message := strings.ReplaceAll(chatHistory[i].Content, fmt.Sprintf("<@%s>", s.State.User.ID), "")
				if id == s.State.User.ID {
					messages = append(messages, map[string]string{
						"role":    "assistant",
						"content": message,
					})
				} else {
					messages = append(messages, map[string]string{
						"role":    "user",
						"content": fmt.Sprintf("%s: %s", nickname[id], message),
					})
				}
			}
		} else {
			fmt.Println("Error getting channel messages:", err)
		}
		if nickname[m.Author.ID] == "" {
			member, err := s.GuildMember(m.GuildID, m.Author.ID)
			if err != nil || member.Nick == "" {
				nickname[m.Author.ID] = m.Author.Username
			} else {
				nickname[m.Author.ID] = member.Nick
			}
		}
		if nickname[s.State.User.ID] == "" {
			member, err := s.GuildMember(m.GuildID, s.State.User.ID)
			if err != nil || member.Nick == "" {
				nickname[s.State.User.ID] = s.State.User.Username
			} else {
				nickname[s.State.User.ID] = member.Nick
			}
		}
		message := strings.ReplaceAll(m.Content, fmt.Sprintf("<@%s>", s.State.User.ID), "")
		messages = append(messages, map[string]string{
			"role":    "user",
			"content": fmt.Sprintf("%s: %s", nickname[m.Author.ID], message),
		})
		if len(systemPrompt) > 0 {
			messages = append(messages, map[string]string{
				"role":    "system",
				"content": systemPrompt,
			})
		}
		send = sendChat
		data = map[string]any{
			"model":      LLM,
			"keep_alive": keepAlive,
			"messages":   messages,
			"stream":     false,
		}
	}
	response, err := send(hostname, data)
	if err != nil {
		response = fmt.Sprint(err)
	}
	if len(response) <= 3 {
		response = "LLM Error"
	}
	stop = true
	for len(response) > 0 {
		var msg string
		if len(response) > 2000 {
			msg = response[:2000]
			response = response[2000:]
		} else {
			msg = response
			response = ""
		}
		_, err := s.ChannelMessageSendComplex(m.ChannelID, &discordgo.MessageSend{
			Content: msg,
			Reference: &discordgo.MessageReference{
				MessageID: m.ID,
			},
		})
		if err != nil {
			fmt.Println("ChannelMessageSendComplex error:", err)
		}
	}
}
func createCommand(s *discordgo.Session) (err error) {
	commandList, err := s.ApplicationCommands(s.State.User.ID, "")
	if err != nil {
		return fmt.Errorf("s.ApplicationCommands() error: %v", err)
	}
	commands := make(map[string]*discordgo.ApplicationCommand)
	for _, v := range commandList {
		commands[v.Name] = v
	}
	version := "1"
	if commands["llm"] == nil {
		_, err = s.ApplicationCommandCreate(s.State.User.ID, "", &discordgo.ApplicationCommand{
			Type:          1,
			ApplicationID: s.State.User.ID,
			Name:          "llm",
			Description:   "change the LLM used by the bot",
			Version:       version,
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "model",
					Description: "give LLM name",
					Required:    true,
				},
			},
		})
		if err != nil {
			return fmt.Errorf("s.ApplicationCommandCreate() error: %v", err)
		}
	}
	if commands["list"] == nil {
		_, err = s.ApplicationCommandCreate(s.State.User.ID, "", &discordgo.ApplicationCommand{
			Type:          1,
			ApplicationID: s.State.User.ID,
			Name:          "list",
			Description:   "list downloaded llms",
			Version:       version,
		})
		if err != nil {
			return fmt.Errorf("s.ApplicationCommandCreate() error: %v", err)
		}
	}
	if commands["current"] == nil {
		_, err = s.ApplicationCommandCreate(s.State.User.ID, "", &discordgo.ApplicationCommand{
			Type:          1,
			ApplicationID: s.State.User.ID,
			Name:          "current",
			Description:   "returns current loaded model",
			Version:       version,
		})
		if err != nil {
			return fmt.Errorf("s.ApplicationCommandCreate() error: %v", err)
		}
	}
	if commands["delete"] == nil {
		_, err = s.ApplicationCommandCreate(s.State.User.ID, "", &discordgo.ApplicationCommand{
			Type:          1,
			ApplicationID: s.State.User.ID,
			Name:          "delete",
			Description:   "delete bot messages",
			Version:       version,
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionInteger,
					Name:        "amount",
					Description: "total amount of messages to delete",
					Required:    true,
				},
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "id",
					Description: "specific message id",
					Required:    false,
				},
			},
		})
		if err != nil {
			return fmt.Errorf("s.ApplicationCommandCreate() error: %v", err)
		}
	}
	if commands["manipulate"] == nil {
		_, err = s.ApplicationCommandCreate(s.State.User.ID, "", &discordgo.ApplicationCommand{
			Type:          1,
			ApplicationID: s.State.User.ID,
			Name:          "manipulate",
			Description:   "send a message as the bot to manipulate future messages",
			Version:       version,
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "msg",
					Description: "message to be resent",
					Required:    true,
				},
			},
		})
		if err != nil {
			return fmt.Errorf("s.ApplicationCommandCreate() error: %v", err)
		}
	}
	if commands["timeout"] == nil {
		_, err = s.ApplicationCommandCreate(s.State.User.ID, "", &discordgo.ApplicationCommand{
			Type:          1,
			ApplicationID: s.State.User.ID,
			Name:          "timeout",
			Description:   "change the time to unload llm from memory",
			Version:       version,
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "time",
					Description: "ex. 5 or 5m for 5 mins",
					Required:    true,
				},
			},
		})
		if err != nil {
			return fmt.Errorf("s.ApplicationCommandCreate() error: %v", err)
		}
	}
	return
}
func commands(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionApplicationCommand {
		return
	}
	llmList := func() (models map[string]string, err error) {
		models = make(map[string]string)
		response, code, err := request(hostname+"api/tags", 60, nil, nil, "GET")
		if err != nil {
			return
		} else if code != 200 {
			err = fmt.Errorf("http status code:%v, response:%v", code, string(response))
			return
		}
		var data map[string]any
		err = json.Unmarshal(response, &data)
		if err != nil {
			err = fmt.Errorf("json parsing error: %s", err)
			return
		}
		defer func() {
			r := recover()
			if r != nil {
				err = fmt.Errorf("%v", r)
			}
		}()
		modelsRaw := data["models"].([]any)
		for _, v := range modelsRaw {
			name := v.(map[string]any)["name"].(string)
			models[name] = name
		}
		return
	}
	switch i.ApplicationCommandData().Name {
	case "llm":
		command := i.ApplicationCommandData().Options[0].StringValue()
		llms, err := llmList()
		if err != nil {
			fmt.Println("llmList error:", err)
			return
		}
		var message string
		if llms[command] == command {
			LLM = command
			message = "LLM changed to " + LLM
		} else {
			message = "LLM not found"
		}
		err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: message,
			},
		})
		if err != nil {
			fmt.Println("InteractionRespond error:", err)
		}
	case "list":
		llms, err := llmList()
		if err != nil {
			fmt.Println("llmList error:", err)
			return
		}
		message := "***Downloaded Models***\n"
		for _, v := range llms {
			message += v + "\n"
		}
		err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: message,
			},
		})
		if err != nil {
			fmt.Println("InteractionRespond error:", err)
		}
	case "timeout":
		command := i.ApplicationCommandData().Options[0].StringValue()
		var message string
		var err error
		var num int
		if strings.Contains(command, "m") {
			command := command[:len(command)-1]
			num, err = strconv.Atoi(command)
		} else {
			num, err = strconv.Atoi(command)
		}
		if err != nil {
			message = "invalid value"
		} else {
			keepAlive = fmt.Sprintf("%dm", num)
			message = fmt.Sprintf("Timeout changed to %s", keepAlive)
		}
		err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: message,
			},
		})
		if err != nil {
			fmt.Println("InteractionRespond error:", err)
		}
	case "current":
		message := fmt.Sprintf("***Current Model***\n%s", LLM)
		err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: message,
			},
		})
		if err != nil {
			fmt.Println("InteractionRespond error:", err)
		}
	case "delete":
		commands := i.ApplicationCommandData().Options
		command1 := commands[0].IntValue()
		var command2 string
		if len(commands) >= 2 {
			command2 = commands[1].StringValue()
		}
		err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: fmt.Sprintf("Deleting %d messages", command1),
			},
		})
		if err != nil {
			fmt.Println("InteractionRespond error:", err)
		}
		err = s.InteractionResponseDelete(i.Interaction)
		if err != nil {
			fmt.Println("InteractionResponseDelete error:", err)
		}
		messages, err := s.ChannelMessages(i.ChannelID, 100, "", "", "")
		if err != nil {
			fmt.Println("ChannelMessages error:", err)
			return
		}
		if len(command2) > 5 {
			err = s.ChannelMessageDelete(i.ChannelID, command2)
			if err != nil {
				fmt.Println("ChannelMessageDelete error:", err)
			}
		}
		for _, v := range messages {
			if v.Author.ID == s.State.User.ID {
				if command1 <= 0 {
					break
				}
				command1--
				err = s.ChannelMessageDelete(i.ChannelID, v.ID)
				if err != nil {
					fmt.Println("ChannelMessageDelete error:", err)
				}
			}
		}
	case "manipulate":
		command := i.ApplicationCommandData().Options[0].StringValue()
		err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: command,
			},
		})
		if err != nil {
			fmt.Println("InteractionRespond error:", err)
		}
	}
}
func onlineService(token string) (err error) {
	bot, err := discordgo.New("Bot " + token)
	if err != nil {
		return fmt.Errorf("discordgo.New() error: %v", err)
	}
	err = bot.Open()
	if err != nil {
		return fmt.Errorf("bot.Open() error: %v", err)
	}
	err = createCommand(bot)
	if err != nil {
		return fmt.Errorf("createCommand() error: %v", err)
	}
	bot.AddHandler(createPrompt)
	bot.AddHandler(commands)
	fmt.Println("Bot Started Successfully")
	online := true
	exit := false
	go func() {
		for !exit {
			_, code, err := request(hostname+"api/tags", 60, nil, nil, "HEAD")
			if err != nil || code != 200 {
				if online {
					err = bot.Close()
					if err == nil {
						online = false
					}
				}
			} else if !online {
				err = bot.Open()
				if err == nil {
					online = true
				}
			}
			time.Sleep(time.Minute * 1)
		}
	}()
	inter := make(chan os.Signal, 1)
	signal.Notify(inter, os.Interrupt, syscall.SIGTERM)
	<-inter
	exit = true
	if online {
		bot.Close()
	}
	return
}
func main() {
	hostname = os.Getenv("HOSTNAME")
	if hostname == "" {
		fmt.Println("hostname not set\n example: 'http://127.0.0.1:11434/'")
		return
	}
	if hostname[len(hostname)-1] != '/' {
		hostname += "/"
	}
	LLM = os.Getenv("MODEL")
	if LLM == "" {
		fmt.Println("LLM not set")
		return
	}
	botToken := os.Getenv("TOKEN")
	if botToken == "" {
		fmt.Println("Discord Token not set")
		return
	}
	systemPrompt = os.Getenv("SYSPROMPT")
	keepAlive = os.Getenv("KEEPLOADED")
	if keepAlive == "" {
		keepAlive = "5m"
	}
	err := onlineService(botToken)
	if err != nil {
		fmt.Printf("onlineService() error: %v\n", err)
		return
	}
}
