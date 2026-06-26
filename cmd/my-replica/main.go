package main

import (
	"log"

	"github.com/pouriatabari/my-replica/internal/app"
)

func main() {
	application := app.New()
	if err := application.Run(); err != nil {
		log.Fatal(err)
	}
}
