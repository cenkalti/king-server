// Command genschema writes the protocol JSON Schema to the given path.
package main

import (
	"log"
	"os"

	"github.com/cenkalti/king-server/protocol"
)

func main() {
	if len(os.Args) != 2 {
		log.Fatal("usage: genschema <output path>")
	}
	b, err := protocol.Schema()
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(os.Args[1], b, 0o644); err != nil {
		log.Fatal(err)
	}
}
