package main

import (
	"context"
	"log"
	"os"
	"os/signal"

	"github.com/lovromazgon/impromptu"
)

func main() {
	i, err := impromptu.New()
	if err != nil {
		log.Fatal(err)
	}

	ctx, _ := signal.NotifyContext(context.Background(), os.Interrupt)
	err = i.Run(ctx)
	if err != nil {
		log.Fatal(err)
	}
}
