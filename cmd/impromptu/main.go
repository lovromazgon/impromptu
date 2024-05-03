package main

import (
	"context"
	"log"
	"os"
	"os/signal"

	"github.com/lovromazgon/impromptu"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	i, err := impromptu.New()
	if err != nil {
		log.Fatal(err)
	}

	err = i.Run(ctx)
	if err != nil {
		log.Fatal(err)
	}
}
