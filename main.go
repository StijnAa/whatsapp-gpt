package main

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"strings"
	_ "github.com/mattn/go-sqlite3"
	"github.com/mdp/qrterminal/v3"
	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
	"github.com/joho/godotenv"
)

type MyClient struct {
	WAClient       *whatsmeow.Client
	eventHandlerID uint32
}

func (mycli *MyClient) register() {
	mycli.eventHandlerID = mycli.WAClient.AddEventHandler(mycli.eventHandler)
}

func goDotEnvVariable(key string) string {

	// load .env file
	err := godotenv.Load(".env")
  
	if err != nil {
		fmt.Println("Error loading .env file")
	}
  
	return os.Getenv(key)
}

func (mycli *MyClient) eventHandler(evt interface{}) {
	switch v := evt.(type) {
	case *events.Message:
		newMessage := v.Message
		phonenumber := goDotEnvVariable("PHONENUMBER")
		var msg string

		if v.Info.IsGroup {
			extendedTextMessage := newMessage.GetExtendedTextMessage()
			text := extendedTextMessage.GetText()
			if !strings.Contains(text, phonenumber) {
				fmt.Println("msg was send to a group but not @", phonenumber)
				return
			}
			fmt.Println("msg was send to a group and @ ", phonenumber)
			msg = strings.ReplaceAll(text, phonenumber, "")
		}else{
			msg = newMessage.GetConversation()
			fmt.Println("msg was send not in a group")
		}

		fmt.Println("msg", msg)

		if msg == "" {
			fmt.Println("msg was empty")
			return
		}
		
		// Make a http request to localhost:5001/chat?q= with the message, and send the response
		// URL encode the message
		urlEncoded := url.QueryEscape(msg)
		url := "http://localhost:5001/chat?q=" + urlEncoded
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
		
		var userJid types.JID
		if v.Info.IsGroup {
			userJid = types.NewJID(v.Info.Chat.User, types.GroupServer)
		} else {
			userJid = types.NewJID(v.Info.Sender.User, types.DefaultUserServer)
		}
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
