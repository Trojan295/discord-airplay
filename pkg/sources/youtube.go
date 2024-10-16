package sources

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/Trojan295/discord-airplay/pkg/bot"
	"golang.org/x/exp/slog"
	"gopkg.in/hraban/opus.v2"
)

const (
	sampleRate = 48000
	channels   = 2

	frameLength = 20 * time.Millisecond
	pcmBufSize  = sampleRate * channels / (time.Second / frameLength)
	opusBufSize = 1024
)

type YoutubeFetcher struct {
	Logger *slog.Logger

	proxy *string
}

type Option func(f *YoutubeFetcher)

func WithProxy(proxy string) Option {
	return func(f *YoutubeFetcher) {
		f.proxy = &proxy
	}
}

func NewYoutubeFetcher(opts ...Option) *YoutubeFetcher {
	f := &YoutubeFetcher{
		Logger: slog.Default(),
	}

	for _, opt := range opts {
		opt(f)
	}

	return f
}

func (s *YoutubeFetcher) LookupSongs(ctx context.Context, input string) ([]*bot.Song, error) {
	ytDlpPrintColumns := []string{"title", "original_url", "is_live", "duration", "thumbnail", "thumbnails"}
	printColumns := strings.Join(ytDlpPrintColumns, ",")

	args := []string{"--print", printColumns, "-U"}

	if strings.HasPrefix(input, "https://") {
		args = append(args, input)
	} else {
		args = append(args, fmt.Sprintf("scsearch:%s", input))
	}

	if s.proxy != nil {
		args = append(args, "--proxy", *s.proxy)
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

		var thumbnailURL *string = nil
		if ytOutLines[linesPerSong*i+4] != "NA" {
			thumbnailURL = &ytOutLines[linesPerSong*i+4]
		} else if ytOutLines[linesPerSong*i+5] != "NA" {
			thumbnail, err := getThumbnail(ytOutLines[linesPerSong*i+5])
			if err != nil {
				s.Logger.Error("failed to get thumbnail", "error", err)
			}
			if thumbnail != nil {
				thumbnailURL = &thumbnail.URL
			}
		}

		song := &bot.Song{
			Type:         "yt-dlp",
			Title:        ytOutLines[linesPerSong*i],
			URL:          ytOutLines[linesPerSong*i+1],
			Playable:     ytOutLines[linesPerSong*i+2] == "False" || ytOutLines[3*i+2] == "NA",
			ThumbnailURL: thumbnailURL,
			Duration:     time.Second * time.Duration(duration),
		}
		if !song.Playable {
			continue
		}

		songs = append(songs, song)
	}

	return songs, nil
}

func (s *YoutubeFetcher) GetAudio(ctx context.Context, song *bot.Song) (<-chan []byte, error) {
	opusCh := make(chan []byte, 500)

	reader, writer := io.Pipe()

	go func() {
		ytArgs := []string{"-U", "-x", "-o", "-", "--force-overwrites", "--http-chunk-size", "100K", "'" + song.URL + "'"}

		if s.proxy != nil {
			ytArgs = append(ytArgs, "--proxy", *s.proxy)
		}

		ffmpegArgs := []string{"-i", "pipe:0"}
		if song.StartPosition > 0 {
			ffmpegArgs = append(ffmpegArgs, "-ss", song.StartPosition.String())
		}
		ffmpegArgs = append(ffmpegArgs, "-f", "s16le", "-ar", "48000", "-ac", "2", "pipe:1")

		downloadCmd := exec.CommandContext(ctx,
			"sh", "-c", fmt.Sprintf("yt-dlp %s | ffmpeg %s",
				strings.Join(ytArgs, " "),
				strings.Join(ffmpegArgs, " ")))

		stderrBuf := &bytes.Buffer{}

		downloadCmd.Stdout = writer
		downloadCmd.Stderr = stderrBuf

		if err := downloadCmd.Run(); err != nil {
			s.Logger.Error("while executing get data pipe", "error", err, "stderr", stderrBuf.String())
		}

		if err := writer.Close(); err != nil {
			s.Logger.Error("while flushing data pipe", "error", err)
		}
	}()

	go func() {
		defer close(opusCh)
		if err := encodeOpus(ctx, reader, opusCh); err != nil {
			s.Logger.Error("while encoding to Opus", "error", err)
		}
	}()

	return opusCh, nil
}

type thumnail struct {
	URL        string `json:"url"`
	Preference int    `json:"preference"`
}

func getThumbnail(thumnailsStr string) (*thumnail, error) {
	thumnailsStr = strings.ReplaceAll(thumnailsStr, "'", "\"")

	var thumbnails []thumnail
	if err := json.Unmarshal([]byte(thumnailsStr), &thumbnails); err != nil {
		return nil, err
	}

	if len(thumbnails) == 0 {
		return nil, nil
	}

	tn := &thumbnails[0]
	for i := range thumbnails {
		t := thumbnails[i]
		if t.Preference > tn.Preference {
			tn = &t
		}
	}

	return tn, nil
}

func encodeOpus(ctx context.Context, dca io.Reader, opusChan chan<- []byte) error {
	enc, err := opus.NewEncoder(sampleRate, channels, opus.AppAudio)
	if err != nil {
		return fmt.Errorf("while creating opus encoder: %w", err)
	}

	for {
		pcmBuf := make([]int16, pcmBufSize)
		if err := binary.Read(dca, binary.LittleEndian, pcmBuf); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return nil
			}

			return fmt.Errorf("while reading PCM: %w", err)
		}

		opusBuf := make([]byte, opusBufSize)

		size, err := enc.Encode(pcmBuf, opusBuf)
		if err != nil {
			return fmt.Errorf("while encoding: %w", err)
		}

		select {
		case <-ctx.Done():
			return nil
		case opusChan <- opusBuf[0:size]:
		}
	}
}
