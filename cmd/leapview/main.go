package main

import (
	"context"
	"log"

	"github.com/Yacobolo/leapview/internal/cli"
)

func main() {
	if err := cli.Execute(context.Background()); err != nil {
		log.Fatal(err)
	}
}
