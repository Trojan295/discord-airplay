package main

import (
	"context"
	"fmt"

	"github.com/Trojan295/discord-airplay/pkg/codec"
	"github.com/Trojan295/discord-airplay/pkg/sources"
	"go.uber.org/zap"
)

var (
	logger         *zap.Logger
	ctx            context.Context
	youtubeFetcher *sources.YoutubeFetcher
)

func main() {
	logger, _ = zap.NewProduction()
	defer logger.Sync()

	ctx = context.Background()
	youtubeFetcher = sources.NewYoutubeFetcher()

	songs, err := youtubeFetcher.LookupSongs(ctx, "https://www.youtube.com/watch?v=AjiSzMFEDgo&pp=ygUNZGVhZG1hdTUgbGl2ZQ%3D%3D")
	//songs, err := youtubeFetcher.LookupSongs(ctx, "https://www.youtube.com/watch?v=AjiSzMFEDgo")
	if err != nil {
		logger.Fatal("failed to get songs", zap.Error(err))
	}

	logger.Info("got songs", zap.Any("songs", songs))

	reader, err := youtubeFetcher.GetDCAData(ctx, songs[0])
	if err != nil {
		logger.Fatal("failed to get dca data", zap.Error(err))
	}

	opusChan := make(chan []byte, 10)
	go func() {
		codec.StreamDCAData(ctx, reader, opusChan, nil)
		close(opusChan)
	}()
	for data := range opusChan {
		fmt.Println(data)
	}
}
