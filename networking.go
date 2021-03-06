package main

type NetworkInterface interface {
	getMessage() Message
	send(*Message)
}

type ChannelNetwork struct {
	sendChan chan<- Message
	recvChan <-chan Message
}

// Ensure ChannelNetwork satisfies NetworkInterface
var _ NetworkInterface = (*ChannelNetwork)(nil)

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

func (cn *ChannelNetwork) send(msg *Message) {
	go func() { cn.sendChan <- *msg }()
}
