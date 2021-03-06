package main

import (
	"encoding/json"
	"fmt"
	"github.com/joho/godotenv"
	"log"
	"net/http"
	"os"
	"stop/frame"
	"stop/jwt"
	"stop/pubsub"
	"stop/remote"
	"stop/ws"
	"strings"
	"time"
)

func main() {
	_ = godotenv.Load()

	serversStr := os.Getenv("SERVERS")
	servers := strings.Split(serversStr, ",")
	var clients []remote.Client
	for _, server := range servers {
		client := remote.NewClient(server)
		if client == nil {
			continue
		}
		clients = append(clients, *client)
	}
	ps := pubsub.NewPubSub()

	go func() {
		for {
			time.Sleep(time.Second * 5)
			if ps.SubscribersNum() == 0 {
				continue
			}
			for _, client := range clients {
				f, err := frame.NewFrame(client)
				if err != nil {
					continue
				}
				msg, err := json.Marshal(f)
				if err != nil {
					continue
				}
				ps.Publish(string(msg))
			}
		}
	}()

	jwtSecret := []byte(os.Getenv("JWT_SECRET"))
	token, err := jwt.GenJwt("stop", jwtSecret, 365)
	if err != nil {
		return
	}
	fmt.Printf(`
       __          
  ___ / /____  ___ 
 (_-</ __/ _ \/ _ \
/___/\__/\___/ .__/
            /_/     Simple and extendable server monitor.
Token: %s
`, token)

	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		client, err := ws.NewClient(w, r)
		if err != nil {
			return
		}
		defer client.Close()

		tokenStr, ok := <-client.ReceiveChan()
		if !ok {
			return
		}

		_, err = jwt.ParseJwt(tokenStr, jwtSecret)
		if err != nil {
			return
		}

		msgChan := make(chan string)
		ps.Subscribe(msgChan)
		for {
			select {
			case <-client.DoneChan():
				ps.Unsubscribe(msgChan)
				return
			case msg := <-msgChan:
				client.SendChan() <- msg
			case <-time.After(1 * time.Second):
			}
		}
	})

	err = http.ListenAndServe(":5566", nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}
