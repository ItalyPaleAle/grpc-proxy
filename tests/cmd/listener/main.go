package main

import (
	"fmt"
	"log"
	"net"
)

func main() {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal("must be able to allocate a port for Listener:", err)
	}

	fmt.Print(lis.Addr().String())
}
