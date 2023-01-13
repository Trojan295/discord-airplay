package main

import (
	"context"
	"fmt"
	"log"

	"github.com/Trojan295/discord-airplay/pkg/sources"
)

func main() {
	ctx := context.Background()

	//term := "Rammstein - Zick Zack (Official Video)"

	streamer := sources.NewYoutubeFetcher()
	song := sources.NewYoutubeSong(streamer).
		WithURL("https://www.youtube.com/watch?v=hBTNyJ33LWI")

	metadata, err := song.GetMetadata(ctx)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(metadata)

	data, err := song.GetDCAData(ctx)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Bytes:", len(data))
}
