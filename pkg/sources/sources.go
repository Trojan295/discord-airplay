package sources

import (
	"context"
	"fmt"

	"github.com/Trojan295/discord-airplay/pkg/bot"
)

func GetAudio(ctx context.Context, song *bot.Song) (<-chan []byte, error) {
	switch song.Type {
	case "yt-dlp":
		return NewYoutubeFetcher().GetAudio(ctx, song)
	}

	return nil, fmt.Errorf("unknown song type %s", song.Type)
}
