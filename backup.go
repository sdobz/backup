package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"sync"
)

func main() {
	runCommand := flag.NewFlagSet("run", flag.ExitOnError)
	clientConfigFlag := runCommand.String("clientConfig", "", "Client configuration")
	serverConfigFlag := runCommand.String("serverConfig", "", "Server configuration")
	if len(os.Args) == 1 {
		fmt.Println("usage: backup <command> [<args>]")
		fmt.Println(" run   Run the backup")
		return
	}

	switch os.Args[1] {
	case "run":
		runCommand.Parse(os.Args[2:])
	default:
		fmt.Printf("%q is not valid command.\n", os.Args[1])
		os.Exit(2)
	}

	if runCommand.Parsed() {
		if *clientConfigFlag == "" {
			log.Print("Please specify client configuration using -clientConfig")
			return
		}
		if *serverConfigFlag == "" {
			log.Print("Please specify server configuration using -serverConfig")
			return
		}
		log.Print("Running backup")

		clientNetwork, serverNetwork := NewChannelNetwork()
		wg := sync.WaitGroup{}
		wg.Add(2)

		go func() {
			log.Print(PerformBackup(*clientConfigFlag, clientNetwork).Error())
			wg.Done()
		}()
		go func() {
			log.Print(ServeBackup(*serverConfigFlag, serverNetwork).Error())
			wg.Done()
		}()
		wg.Wait()
	}
}
