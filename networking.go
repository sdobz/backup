package main

import "log"

type NetworkInterface interface {
	getMessage() Message
	send(Message)
}

type ChannelNetwork struct {
	sendChan chan<- Message
	recvChan <-chan Message
}

func NewChannelNetwork() (client *ChannelNetwork, server *ChannelNetwork) {
	clientChan := make(chan Message)
	serverChan := make(chan Message)

	client = &ChannelNetwork{
		sendChan: clientChan,
		recvChan: serverChan,
	}

	server = &ChannelNetwork{
		sendChan: serverChan,
		recvChan: clientChan,
	}

	return client, server
}

func (cn *ChannelNetwork) getMessage() Message {
	return <-cn.recvChan
}

func (cn *ChannelNetwork) send(msg Message) {
	log.Printf("Sending: %v", msg)
	go func() { cn.sendChan <- msg }()
}
