package sse

import (
	"net/http"
	"encoding/json"
)

// ----------------------------------------------------------------------------------
//  types
// ----------------------------------------------------------------------------------

type Group struct {
	broker *Broker
	name string
}


// ----------------------------------------------------------------------------------
//  members
// ----------------------------------------------------------------------------------

func (g *Group) Subscribe() (int, chan []byte) {
	g.broker.mutex.Lock()
	defer g.broker.mutex.Unlock()

	// create the new channel and place it in the client map
	ch := make(chan []byte)
	id := len(g.broker.clients[g.name])
	g.broker.clients[g.name][id] = ch

	return id, ch
}

func (g *Group) Unsubscribe(id int) {
	g.broker.mutex.Lock()
	defer g.broker.mutex.Unlock()

	// only delete the channel if it exists in the client map
	ch := g.broker.clients[g.name][id]
	if ch == nil {
		return
	}

	// close and delete the actual channel
	close(ch)
	delete(g.broker.clients[g.name], id)
}

func (g *Group) Broadcast(event string, obj interface{}) {
	g.broker.mutex.Lock()
	defer g.broker.mutex.Unlock()

	// check if group does exist
	clients := g.broker.clients[g.name]
	if clients == nil {
		return
	}

	// convert the supplied object obj to a json string
	js, err := json.Marshal(obj)
	if err != nil {
    	return
  	}

	// send the event data to every client in the group
	for _,ch := range clients {
		ch <- []byte("event: " + event + "\r\n" + "data: " + string(js) + "\r\n\r\n")
	}
}

func (g *Group) Finish() {
	g.broker.mutex.Lock()
	defer g.broker.mutex.Unlock()

	// close all channels for this group
	for _,ch := range g.broker.clients[g.name] {
		close(ch)
	}

	// delete the whole client map for this group
	delete(g.broker.clients, g.name)
}

func (g *Group) Handle(rw http.ResponseWriter, req *http.Request) {
	// is streaming even possible?
	flusher, ok := rw.(http.Flusher)
	if !ok {
		http.Error(rw, "Streaming unsupported!", http.StatusInternalServerError)
		return
	}

	// setup headers for SSE
	rw.Header().Set("Content-Type", "text/event-stream")
	rw.Header().Set("Cache-Control", "no-cache")
	rw.Header().Set("Connection", "keep-alive")

	// get a channel
	id, ch := g.Subscribe()

	// we want to be informed when the client closes the connection
	notify := rw.(http.CloseNotifier).CloseNotify()
	go func() {
		// wait until close notification is received
		<-notify

		// unsubscribe from the sse group
		g.Unsubscribe(id)

	}()

	// send all data received by our event channel
	// to the client via http
	for data, valid := <- ch; valid; data, valid = <- ch {	
		rw.Write(data)
		flusher.Flush()
	}
}