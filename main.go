package main

import (
	"crypto/rand"
	"fmt"
)

func main() {
	salt := make([]byte, 32)
	rand.Read(salt)
	fmt.Printf("%x\n", salt)
}
