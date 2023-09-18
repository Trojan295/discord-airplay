package main

import (
	"context"
	"io"
	"os"

	"github.com/Trojan295/discord-airplay/pkg/sources"
)

func main() {
	ctx := context.Background()

	ytFetcher := sources.NewYoutubeFetcher()
	songs, err := ytFetcher.LookupSongs(ctx, "muncipal waste")
	if err != nil {
		panic(err)
	}

	reader, err := ytFetcher.GetDCAData(ctx, songs[0])
	if err != nil {
		panic(err)
	}

	_, err = io.Copy(os.Stdout, reader)
	if err != nil {
		panic(err)
	}
}
