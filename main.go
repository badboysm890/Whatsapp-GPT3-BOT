package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"

	_ "github.com/mattn/go-sqlite3"
	"github.com/mdp/qrterminal/v3"
	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
)

type MyClient struct {
	WAClient       *whatsmeow.Client
	eventHandlerID uint32
}

type Config struct {
	PhoneNumbers []string
	HotWords     []string
	Author       string
}

func (mycli *MyClient) register() {
	mycli.eventHandlerID = mycli.WAClient.AddEventHandler(mycli.eventHandler)
}

// contains checks if a string is in an array of strings
func contains(arr []string, str string) bool {

	// if the array is empty, return true
	if len(arr) == 0 {
		return true
	}

	for _, a := range arr {
		if a == str {
			return true
		}
	}
	return false
}

func readConfig() Config {
	// Read the config file
	file, err := os.Open("config.json")
	if err != nil {
		fmt.Println("Error opening config file:", err)
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	config := Config{}
	err = decoder.Decode(&config)
	if err != nil {
		fmt.Println("Error decoding config file:", err)
	}
	return config
}

func (mycli *MyClient) eventHandler(evt interface{}) {
	switch v := evt.(type) {
	case *events.Message:
		newMessage := v.Message
		msg := newMessage.GetConversation()
		// some messages are empty, so we need to check for that

		msg_raw := newMessage.GetExtendedTextMessage()
		fmt.Println("Message from - Raw:", v.Info.Sender.User, "->", msg_raw)

		fmt.Println("Message from - Conv:", v.Info.Sender.User, "->", msg)
		fmt.Println("---------------------->", msg)
		if msg == "" {
			msg = msg_raw.GetText()
		}

		cfgData := readConfig()

		phoneNumbers := cfgData.PhoneNumbers

		if !contains(phoneNumbers, v.Info.Sender.User) {
			return
		}

		// if !contains(cfgData.HotWords, msg) {
		// 	return
		// }

		// remove the hotwords from the message
		// for _, hotword := range cfgData.HotWords {
		// 	msg = msg[len(hotword):]
		// }

		// Make a http request to localhost:5001/chat?q= with the message, and send the response
		// URL encode the message
		urlEncoded := url.QueryEscape(msg)

		url := "http://localhost:5002/chat?q=" + urlEncoded
		// Make the request
		resp, err := http.Get(url)
		if err != nil {
			fmt.Println("Error making request:", err)
			return
		}
		// Read the response
		buf := new(bytes.Buffer)
		buf.ReadFrom(resp.Body)
		newMsg := buf.String()
		// encode out as a string
		response := &waProto.Message{Conversation: proto.String(string(newMsg))}
		fmt.Println("Response:", response)
		userJid := types.NewJID(v.Info.Sender.User, types.DefaultUserServer)
		mycli.WAClient.SendMessage(context.Background(), userJid, "", response)
	}
}

func main() {
	dbLog := waLog.Stdout("Database", "DEBUG", true)
	// Make sure you add appropriate DB connector imports, e.g. github.com/mattn/go-sqlite3 for SQLite
	container, err := sqlstore.New("sqlite3", "file:examplestore.db?_foreign_keys=on", dbLog)
	if err != nil {
		panic(err)
	}
	// If you want multiple sessions, remember their JIDs and use .GetDevice(jid) or .GetAllDevices() instead.
	deviceStore, err := container.GetFirstDevice()
	if err != nil {
		panic(err)
	}
	clientLog := waLog.Stdout("Client", "DEBUG", true)
	client := whatsmeow.NewClient(deviceStore, clientLog)
	// add the eventHandler
	mycli := &MyClient{WAClient: client}
	mycli.register()

	if client.Store.ID == nil {
		// No ID stored, new login
		qrChan, _ := client.GetQRChannel(context.Background())
		err = client.Connect()
		if err != nil {
			panic(err)
		}
		for evt := range qrChan {
			if evt.Event == "code" {
				// Render the QR code here
				// e.g. qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
				// or just manually `echo 2@... | qrencode -t ansiutf8` in a terminal
				qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
				//				fmt.Println("QR code:", evt.Code)
			} else {
				fmt.Println("Login event:", evt.Event)
			}
		}
	} else {
		// Already logged in, just connect
		err = client.Connect()
		if err != nil {
			panic(err)
		}
	}

	// Listen to Ctrl+C (you can also do something else that prevents the program from exiting)
	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c

	client.Disconnect()
}
