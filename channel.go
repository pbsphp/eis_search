package main

// Closeable channel.
type Channel struct {
	C         chan *Result
	Closed    bool
	closeChan chan struct{}
}

// Create new closeable channel.
func NewChannel() *Channel {
	return &Channel{
		C:         make(chan *Result),
		closeChan: make(chan struct{}),
	}
}

// Try to write Result data into channel.
// Return true on success and false if channel is closed by user.
func (ch *Channel) Write(result *Result) bool {
	select {
	case ch.C <- result:
		return true
	case <-ch.closeChan:
		return false
	}
}

// Close channel by user.
func (ch *Channel) ForceClose() {
	ch.Closed = true
	close(ch.closeChan)
}
