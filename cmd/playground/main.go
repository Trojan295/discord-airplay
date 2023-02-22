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
	//playlist
	url := "https://www.youtube.com/watch?v=u7K72X4eo_s&list=RDEMHk07TM01OFpLd0Sok9_H2w"

	fetcher := sources.NewYoutubeFetcher()
	songs, err := fetcher.LookupSongs(ctx, url)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(songs)

}
