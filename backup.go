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
			fmt.Println("Please specify client configuration using -clientConfig")
			return
		}

		clientNetwork, serverNetwork := NewChannelNetwork()
		wg := sync.WaitGroup{}

		go func() {
			wg.Add(1)
			log.Printf(PerformBackup(*clientConfigFlag, clientNetwork).Error())
			wg.Done()
		}()
		go func() {
			wg.Add(1)
			log.Print(ServeBackup(*serverConfigFlag, serverNetwork).Error())
			wg.Done()
		}()
		wg.Wait()
	}
}
