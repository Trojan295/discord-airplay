package sources

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"os/exec"
	"strings"

	"github.com/Trojan295/discord-airplay/pkg/bot"
)

const (
	downloadBuffer = 100 * 1024 // 100 KiB
)

type YoutubeFetcher struct {
}

func NewYoutubeFetcher() *YoutubeFetcher {
	return &YoutubeFetcher{}
}

func (s *YoutubeFetcher) LookupSongs(ctx context.Context, input string) ([]bot.Song, error) {
	args := []string{"--print", "title,original_url,is_live", "--flat-playlist"}

	if strings.HasPrefix(input, "https://") {
		args = append(args, input)
	} else {
		args = append(args, fmt.Sprintf("ytsearch:%s", input))
	}

	ytCmd := exec.CommandContext(ctx, "yt-dlp", args...)

	ytOutBuf := &bytes.Buffer{}
	ytCmd.Stdout = ytOutBuf

	if err := ytCmd.Run(); err != nil {
		return nil, fmt.Errorf("while executing yt-dlp command to get metadata: %w", err)
	}

	ytOutLines := strings.Split(ytOutBuf.String(), "\n")
	songCount := len(ytOutLines) / 3

	songs := make([]bot.Song, 0, songCount)
	for i := 0; i < songCount; i++ {
		metadata := bot.SongMetadata{
			Title:    ytOutLines[3*i],
			URL:      ytOutLines[3*i+1],
			Playable: ytOutLines[3*i+2] == "False" || ytOutLines[3*i+2] == "NA",
		}
		if !metadata.Playable {
			continue
		}

		song := NewYoutubeSong(metadata, s)
		songs = append(songs, song)
	}

	return songs, nil
}

func (s *YoutubeFetcher) GetDCAData(ctx context.Context, song *YoutubeSong, writer io.Writer) error {
	ytArgs := s.getYTdlpGetDataArgs(song)
	ytCmd := strings.Join(append([]string{"yt-dlp"}, ytArgs...), " ")

	dcaCmd := exec.CommandContext(ctx, "sh", "-c", fmt.Sprintf("%s | ffmpeg -i pipe: -f s16le -ar 48000 -ac 2 pipe:1 | dca", ytCmd))

	bw := bufio.NewWriterSize(writer, downloadBuffer)
	dcaCmd.Stdout = bw

	if err := dcaCmd.Run(); err != nil {
		return fmt.Errorf("while executing get DCA data pipe: %w", err)
	}

	if err := bw.Flush(); err != nil {
		return fmt.Errorf("while flushing buffer: %w", err)
	}

	return nil
}

func (s *YoutubeFetcher) getYTdlpGetDataArgs(song *YoutubeSong) []string {
	return []string{"-x", "-o", "-", "--force-overwrites", song.metadata.URL}
}

type YoutubeSong struct {
	metadata bot.SongMetadata
	fetcher  *YoutubeFetcher

	dcaData *bytes.Buffer
}

func NewYoutubeSong(metadata bot.SongMetadata, streamer *YoutubeFetcher) *YoutubeSong {
	return &YoutubeSong{
		metadata: metadata,
		fetcher:  streamer,

		dcaData: nil,
	}
}

func (s *YoutubeSong) GetMetadata() *bot.SongMetadata {
	return &s.metadata
}

func (s *YoutubeSong) GetHumanName() string {
	if s.metadata.Title != "" {
		return s.metadata.Title
	}

	return s.metadata.URL
}

func (s *YoutubeSong) GetDCAData(ctx context.Context) (io.Reader, error) {
	reader, writer := io.Pipe()

	go func() {
		if err := s.fetcher.GetDCAData(ctx, s, writer); err != nil {
			log.Printf("failed to get DCA data: %v", err)
		}
		writer.Close()
	}()

	return reader, nil
}
