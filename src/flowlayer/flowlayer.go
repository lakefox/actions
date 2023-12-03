package flowlayer

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"net/http"

	"github.com/gorilla/websocket"
)

// <----------------------------------------------------------------------------------------------> //

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

type Event struct {
	User    Socket
	Data    map[string]interface{}
	Channel float64
}

type Channel struct {
	ID        float64
	Socket    Socket
	Callbacks map[string]func(any)
}

type Socket struct {
	ToID      float64
	ID        float64
	Read      chan Event
	Write     chan string
	Callbacks map[string]func(any)
	Channels  map[float64]Channel
	Routes    map[string]func(any) any
	Requests  []func(any)
}

// <----------------------------------------------------------------------------------------------> //

func Serve(w http.ResponseWriter, r *http.Request) *Socket {
	conn, _ := upgrader.Upgrade(w, r, nil) // error ignored for sake of simplicity
	// defer conn.Close()

	user := auth(conn)

	user.Write <- `{"type": "connect"}`
	return user
}

func Connect(url string) *Socket {
	conn, _, _ := websocket.DefaultDialer.Dial(url, nil)

	user := auth(conn)
	return user
}

func auth(conn *websocket.Conn) *Socket {

	connId := math.Round(rand.Float64() * 10000000)

	readC := make(chan Event)
	writeC := make(chan string)
	callbacks := make(map[string]func(any))
	channels := make(map[float64]Channel)
	routes := make(map[string]func(any) any)

	user := Socket{
		ToID:      connId,
		Read:      readC,
		Write:     writeC,
		Callbacks: callbacks,
		Channels:  channels,
		Routes:    routes,
		Requests:  []func(any){},
	}

	go func() {
		for {
			// Read message from browser
			_, msg, err := conn.ReadMessage()
			if err != nil {
				fmt.Println(err)
				return
			}

			var data map[string]interface{}
			if err := json.Unmarshal(msg, &data); err != nil {
				// Error decoding JSON
				fmt.Println(err, string(msg))
			} else {
				handled := *handleMessage(&user, data)

				if handled.Data != nil {
					if handled.Data["authd"] == true {
						cb := user.Callbacks["open"]
						if cb != nil {
							cb(data)
						}
					} else {
						t := handled.Data["data"].(map[string]interface{})
						dataType := t["type"].(string)
						if handled.Channel != 0 {
							cb := handled.User.Channels[handled.Channel].Callbacks[dataType]
							if cb != nil {
								cb(t)
							}
						} else {
							cb := handled.User.Callbacks[dataType]
							if cb != nil {
								cb(t)
							}
						}
					}

				}
			}
		}
	}()

	// Start a goroutine to send messages to the WebSocket connection
	go func() {
		for {
			select {
			case message := <-user.Write:
				err := conn.WriteMessage(websocket.TextMessage, []byte(message))
				if err != nil {
					fmt.Println("write:", err)
					return
				}
			}
		}
	}()
	return &user
}

func handleMessage(user *Socket, data map[string]interface{}) *Event {
	id := data["id"]
	rt := &Event{}

	if id == nil || data["type"] == "id" {
		if data["type"] == "connect" {
			if data["ident"] == true {
				// After you initiated the handshake they are asking you assign them an ID
				msg := fmt.Sprintf(`{"type": "id", "id": %f}`, user.ToID)
				user.Write <- msg
			} else {
				// Someone else initiated the handshake
				// Assign them an ID
				msg := fmt.Sprintf(`{"type": "id", "id": %f}`, user.ToID)
				user.Write <- msg

				// Ask them to connect
				req := `{"type": "connect", "ident": true}`
				user.Write <- req
			}
		} else if data["type"] == "id" {
			user.ID = data["id"].(float64)
			connect := make(map[string]interface{})
			connect["authd"] = true
			rt = &Event{
				User: *user,
				Data: connect,
			}
		}
	} else {
		if data["type"] == "data" {
			rt = &Event{
				User: *user,
				Data: data,
			}

			ch, e := data["channel"]
			if e {
				rt.Channel = ch.(float64)
			}

		} else if data["type"] == "fetch" && data["status"] != nil {
			if data["status"].(float64) == 0 {
				g := data["data"].(map[string]interface{})
				out := user.Routes[g["type"].(string)](g["data"])
				d := DATA{
					Type: g["type"].(string),
					Data: out,
				}

				msg := FETCH{
					Type:   "fetch",
					ID:     user.ToID,
					Status: 1,
					Index:  data["index"].(float64),
					Data:   d,
				}

				jsonData, err := json.Marshal(msg)
				if err != nil {
					fmt.Println("JSON marshaling error:", err)
				}
				user.Write <- string(jsonData)
			} else if data["status"].(float64) == 1 {
				fmt.Println(data["index"].(float64))
				d := data["data"].(map[string]interface{})
				user.Requests[int(data["index"].(float64))](d["data"])
			}

		} else if data["type"] == "open" {
			// If the channel is defined then create a channel with that id else create a new socket or "user"
			if data["channel"] != nil {
				callbacks := make(map[string]func(any))
				cid := data["channel"].(float64)

				channel := Channel{
					ID:        cid,
					Callbacks: callbacks,
					Socket:    *user,
				}

				user.Channels[cid] = channel

				cb := user.Callbacks["channel"]
				if cb != nil {
					cb(user.Channels[cid])
				}
			}
		}
	}
	return rt
}

// <----------------------------------------------------------------------------------------------> //

type FETCH struct {
	Type   string  `json:"type"`
	ID     float64 `json:"id"`
	Status float64 `json:"status"`
	Index  float64 `json:"index"`
	Data   DATA    `json:"data"`
}

type DATA struct {
	Type string `json:"type"`
	Data any    `json:"data"`
}

type JSONC struct {
	Type    string  `json:"type"`
	ID      float64 `json:"id"`
	Channel float64 `json:"channel"`
	Data    DATA    `json:"data"`
}
type JSON struct {
	Type string  `json:"type"`
	ID   float64 `json:"id"`
	Data DATA    `json:"data"`
}

func (e *Socket) Emit(event string, message any) {
	d := DATA{
		Type: event,
		Data: message,
	}

	msg := JSON{
		Type: "data",
		ID:   e.ToID,
		Data: d,
	}

	jsonData, err := json.Marshal(msg)
	if err != nil {
		fmt.Println("JSON marshaling error:", err)
		return
	}
	e.Write <- string(jsonData)
}

func (e *Channel) Emit(event string, message any) {
	d := DATA{
		Type: event,
		Data: message,
	}

	msg := JSONC{
		Type:    "data",
		ID:      e.Socket.ToID,
		Data:    d,
		Channel: e.ID,
	}

	jsonData, err := json.Marshal(msg)
	if err != nil {
		fmt.Println("JSON marshaling error:", err)
		return
	}
	e.Socket.Write <- string(jsonData)
}

func (e *Socket) CreateChannel() {
	msg := JSONC{
		Type:    "open",
		ID:      e.ToID,
		Channel: math.Round(rand.Float64() * 10000000),
	}

	jsonData, err := json.Marshal(msg)
	if err != nil {
		fmt.Println("JSON marshaling error:", err)
		return
	}
	e.Write <- string(jsonData)
}

func (e *Socket) Fetch(event string, data any, callback func(any)) {
	d := DATA{
		Type: event,
		Data: data,
	}

	msg := FETCH{
		Type:   "fetch",
		ID:     e.ToID,
		Status: 0,
		Index:  float64(len(e.Requests)),
		Data:   d,
	}

	e.Requests = append(e.Requests, callback)

	jsonData, err := json.Marshal(msg)
	if err != nil {
		fmt.Println("JSON marshaling error:", err)
		return
	}
	e.Write <- string(jsonData)
}

func (e *Socket) Route(event string, callback func(any) any) {
	_, exists := e.Routes[event]

	if !exists {
		e.Routes[event] = callback
	}
}

func (e *Socket) On(event string, callback func(any)) {
	_, exists := e.Callbacks[event]

	if !exists {
		e.Callbacks[event] = callback
	}
}

func (e *Channel) On(event string, callback func(any)) {
	_, exists := e.Callbacks[event]

	if !exists {
		e.Callbacks[event] = callback
	}
}

// <----------------------------------------------------------------------------------------------> //
