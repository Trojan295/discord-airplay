package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Trojan295/discord-airplay/pkg/sources"
)

func main() {
	token := os.Getenv("AIR_OPENAITOKEN")

	dj := sources.NewChatGPTPlaylistGenerator(token)

	description := os.Args[1]
	songs, err := dj.GeneratePlaylist(context.Background(), &sources.PlaylistParams{
		Description: description,
		Length:      5,
	})

	fmt.Println(songs, err)
}
