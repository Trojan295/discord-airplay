package sources

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/Trojan295/discord-airplay/pkg/bot"
)

const (
	downloadBuffer = 100 * 1024 // 100 KiB
)

type YoutubeFetcher struct{}

func NewYoutubeFetcher() *YoutubeFetcher {
	return &YoutubeFetcher{}
}

func (s *YoutubeFetcher) LookupSongs(ctx context.Context, input string) ([]*bot.Song, error) {
	ytDlpPrintColumns := []string{"title", "original_url", "is_live", "duration"}
	printColumns := strings.Join(ytDlpPrintColumns, ",")

	args := []string{"--print", printColumns, "--flat-playlist", "-U"}

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

	linesPerSong := len(ytDlpPrintColumns)
	ytOutLines := strings.Split(ytOutBuf.String(), "\n")
	songCount := len(ytOutLines) / linesPerSong

	songs := make([]*bot.Song, 0, songCount)
	for i := 0; i < songCount; i++ {
		duration, _ := strconv.ParseFloat(ytOutLines[linesPerSong*i+3], 32)

		song := &bot.Song{
			Type:     "yt-dlp",
			Title:    ytOutLines[linesPerSong*i],
			URL:      ytOutLines[linesPerSong*i+1],
			Playable: ytOutLines[linesPerSong*i+2] == "False" || ytOutLines[3*i+2] == "NA",
			Duration: time.Second * time.Duration(duration),
		}
		if !song.Playable {
			continue
		}

		songs = append(songs, song)
	}

	return songs, nil
}

func (s *YoutubeFetcher) GetDCAData(ctx context.Context, song *bot.Song) (io.Reader, error) {
	reader, writer := io.Pipe()

	go func(w io.WriteCloser) {
		defer w.Close()

		ytArgs := []string{"-U", "-x", "-o", "-", "--force-overwrites", "--http-chunk-size", "100K", "'" + song.URL + "'"}

		ffmpegArgs := []string{"-i", "pipe:0"}
		if song.StartPosition > 0 {
			ffmpegArgs = append(ffmpegArgs, "-ss", song.StartPosition.String())
		}
		ffmpegArgs = append(ffmpegArgs, "-f", "s16le", "-ar", "48000", "-ac", "2", "pipe:1")

		downloadCmd := exec.CommandContext(ctx,
			"sh", "-c", fmt.Sprintf("yt-dlp %s | ffmpeg %s | dca",
				strings.Join(ytArgs, " "),
				strings.Join(ffmpegArgs, " ")))

		bw := bufio.NewWriterSize(writer, downloadBuffer)
		downloadCmd.Stdout = bw

		if err := downloadCmd.Run(); err != nil {
			log.Printf("while executing get DCA data pipe: %v", err)
		}

		if err := bw.Flush(); err != nil {
			log.Printf("while flushing DCA data pipe: %v", err)
		}
	}(writer)

	return reader, nil
}
