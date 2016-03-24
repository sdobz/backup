package main

import (
	"flag"
	"fmt"
	"os"
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

		if *serverConfigFlag == "" {
			fmt.Println("Please specify server configuration using -serverConfig")
			return
		}

		clientNetwork, serverNetwork := NewChannelNetwork()
		go ServeBackup(*serverConfigFlag, serverNetwork)
		PerformBackup(*clientConfigFlag, clientNetwork)
	}
}
