package main

func main() {
	clientComm, serverComm := NewChannelNetwork()

	go func() {
		PerformBackup(BackupClientConfig{}, clientComm)
	}()
	ServeBackup(BackupServerConfig{}, serverComm)
}
