package main

import (
	"go-smtp/cmd" // Updated import path
)

func main() {
	cmd.Execute()
}

// All previous logic (loading config, starting listener, etc.)
// has been moved to cmd/start.go
//
// package main
//
// import (
// 	"fmt"
// 	"log"
// 	"net"
// 	// "strings" // No longer needed directly in main
// 	// "bufio"   // No longer needed directly in main
//
// 	"go-smtp/config" // Added import for config
// 	"go-smtp/smtp"
// )
//
// func main() {
// 	// Load configuration
// 	cfg, err := config.LoadConfig("config.toml")
// 	if err != nil {
// 		log.Fatalf("Failed to load configuration: %v", err)
// 	}
//
// 	listenAddr := fmt.Sprintf("%s:%d", cfg.ListenInterface, cfg.ListenPort)
// 	listener, err := net.Listen("tcp", listenAddr)
// 	if err != nil {
// 		log.Fatalf("Error listening on %s: %s", listenAddr, err.Error())
// 		return
// 	}
// 	defer listener.Close()
// 	log.Printf("SMTP server listening on %s", listenAddr)
//
// 	for {
// 		conn, err := listener.Accept()
// 		if err != nil {
// 			log.Printf("Error accepting connection: %s", err.Error())
// 			// Depending on the error, we might want to continue or break
// 			// For now, just log and continue accepting
// 			continue
// 		}
// 		go smtp.HandleConnection(conn, cfg) // Pass config to HandleConnection
// 	}
// }
//
//
// // handleConnection function has been moved to smtp/server.go
