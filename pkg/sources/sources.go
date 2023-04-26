package sources

import (
	"context"
	"fmt"
	"io"

	"github.com/Trojan295/discord-airplay/pkg/bot"
)

func GetDCAData(ctx context.Context, song *bot.Song) (io.Reader, error) {
	switch song.Type {
	case "yt-dlp":
		return NewYoutubeFetcher().GetDCAData(ctx, song)
	}

	return nil, fmt.Errorf("unknown song type %s", song.Type)
}
