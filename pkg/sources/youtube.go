package sources

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/Trojan295/discord-airplay/pkg/bot"
)

type YoutubeFetcher struct {
}

func NewYoutubeFetcher() *YoutubeFetcher {
	return &YoutubeFetcher{}
}

func (s *YoutubeFetcher) GetSongMetadata(ctx context.Context, song *YoutubeSong) (*bot.SongMetadata, error) {
	ytArgs := s.getYTdlpGetMetadataArgs(song)
	ytCmd := exec.CommandContext(ctx, "yt-dlp", ytArgs...)

	ytOutBuf := &bytes.Buffer{}
	ytCmd.Stdout = ytOutBuf

	if err := ytCmd.Run(); err != nil {
		return nil, fmt.Errorf("while executing yt-dlp command to get metadata: %w", err)
	}

	ytOutLines := strings.Split(ytOutBuf.String(), "\n")

	return &bot.SongMetadata{
		Title:    ytOutLines[0],
		URL:      ytOutLines[1],
		Playable: ytOutLines[2] == "False",
	}, nil
}

func (s *YoutubeFetcher) GetDcaData(ctx context.Context, song *YoutubeSong) ([]byte, error) {
	opusFile, err := os.CreateTemp("", "youtube-*.opus")
	if err != nil {
		return nil, fmt.Errorf("while creating temp file: %w", err)
	}
	defer opusFile.Close()
	defer os.Remove(opusFile.Name())

	ytArgs := s.getYTdlpGetDataArgs(opusFile.Name(), song)
	ytCmd := exec.CommandContext(ctx, "yt-dlp", ytArgs...)

	if err := ytCmd.Run(); err != nil {
		return nil, fmt.Errorf("while executing yt-dlp command: %w", err)
	}

	dcaFile, err := os.CreateTemp("", "youtube-*.dca")
	if err != nil {
		return nil, fmt.Errorf("while creating temp file: %w", err)
	}
	defer dcaFile.Close()
	defer os.Remove(dcaFile.Name())

	buf := bytes.Buffer{}

	dcaCmd := exec.CommandContext(ctx, "sh", "-c", fmt.Sprintf("ffmpeg -i %s -f s16le -ar 48000 -ac 2 pipe:1 | dca", opusFile.Name()))
	dcaCmd.Stdout = &buf

	if err := dcaCmd.Run(); err != nil {
		return nil, fmt.Errorf("while executing ffmpeg command: %w", err)
	}

	return buf.Bytes(), err
}

func (s *YoutubeFetcher) getYTdlpGetDataArgs(outputFilepath string, song *YoutubeSong) []string {
	args := []string{"-x", "-o", outputFilepath, "--force-overwrites"}

	if song.metadata.URL != "" {
		args = append(args, song.metadata.URL)
	} else {
		args = append(args, fmt.Sprintf("ytsearch:%s", *song.searchTerm))
	}

	return args
}

func (s *YoutubeFetcher) getYTdlpGetMetadataArgs(song *YoutubeSong) []string {
	args := []string{"--print", "title,original_url,is_live"}

	if song.metadata.URL != "" {
		args = append(args, song.metadata.URL)
	} else {
		args = append(args, fmt.Sprintf("ytsearch:%s", *song.searchTerm))
	}

	return args
}

func ParseYoutubeInput(input string, streamer *YoutubeFetcher) *YoutubeSong {
	song := NewYoutubeSong(streamer)

	if strings.HasPrefix(input, "https://www.youtube.com") {
		return song.WithURL(input)
	}

	return song.WithSearchTerm(input)
}

type YoutubeSong struct {
	metadata bot.SongMetadata

	searchTerm *string
	data       []byte

	streamer *YoutubeFetcher

	dataMutex     sync.Mutex
	metadataMutex sync.Mutex

	metadataFetched bool
}

func NewYoutubeSong(streamer *YoutubeFetcher) *YoutubeSong {
	return &YoutubeSong{
		streamer: streamer,

		data: nil,

		dataMutex:     sync.Mutex{},
		metadataMutex: sync.Mutex{},

		metadataFetched: false,
	}
}

func (s *YoutubeSong) WithURL(url string) *YoutubeSong {
	s.metadata.URL = url
	return s
}

func (s *YoutubeSong) WithSearchTerm(term string) *YoutubeSong {
	s.searchTerm = &term
	return s
}

func (s *YoutubeSong) GetHumanName() string {
	if s.metadata.Title != "" {
		return s.metadata.Title
	}

	if s.metadata.URL != "" {
		return s.metadata.URL
	}

	return *s.searchTerm
}

func (s *YoutubeSong) GetMetadata(ctx context.Context) (*bot.SongMetadata, error) {
	s.metadataMutex.Lock()
	defer s.metadataMutex.Unlock()

	if s.metadataFetched {
		return &s.metadata, nil
	}

	metadata, err := s.streamer.GetSongMetadata(ctx, s)
	if err != nil {
		return nil, err
	}

	s.metadata = *metadata
	s.metadataFetched = true

	return &s.metadata, nil
}

func (s *YoutubeSong) GetDCAData(ctx context.Context) ([]byte, error) {
	s.dataMutex.Lock()
	defer s.dataMutex.Unlock()
	if s.data != nil {
		return s.data, nil
	}

	data, err := s.streamer.GetDcaData(ctx, s)
	if err != nil {
		return nil, err
	}

	s.data = data
	return s.data, nil
}
