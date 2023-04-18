package sources

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/sashabaranov/go-openai"
)

type ChatGPTPlaylistGenerator struct {
	openAIClient *openai.Client
}

func NewChatGPTPlaylistGenerator(token string) *ChatGPTPlaylistGenerator {
	client := openai.NewClient(token)
	return &ChatGPTPlaylistGenerator{
		openAIClient: client,
	}
}

type PlaylistParams struct {
	Description string
	Length      int
}

type PlaylistResponse struct {
	Intro    string
	Playlist []string
}

func (g *ChatGPTPlaylistGenerator) GeneratePlaylist(ctx context.Context, params *PlaylistParams) (*PlaylistResponse, error) {
	if params == nil {
		return nil, fmt.Errorf("playlist params are nil")
	}

	if params.Length < 1 {
		params.Length = 10
	}

	resp, err := g.openAIClient.CreateChatCompletion(
		context.Background(),
		openai.ChatCompletionRequest{
			Model: openai.GPT3Dot5Turbo,
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleSystem,
					Content: "I want you to act as a DJ. I will provide you with a description of a playlist and number of songs, and you will create it for me. You should output the list of songs, each in a new line with artist and title. Add also some nice introduction before the song list. Do not include any additional information or description, simply output: <introduction>\n<song number>. <artist> - <title>",
				},
				{
					Role:    openai.ChatMessageRoleUser,
					Content: fmt.Sprintf("Create for me a playlist of %d songs of %s", params.Length, params.Description),
				},
			},
		},
	)
	if err != nil {
		return nil, fmt.Errorf("while creating chat completion: %w", err)
	}

	lines := strings.Split(resp.Choices[0].Message.Content, "\n")

	regex := regexp.MustCompile(`^\d+\.(.+)$`)

	playlist := make([]string, 0)
	for _, line := range lines {
		matches := regex.FindStringSubmatch(line)
		if matches == nil {
			continue
		}

		playlist = append(playlist, strings.TrimSpace(matches[1]))
	}

	return &PlaylistResponse{
		Intro:    lines[0],
		Playlist: playlist,
	}, nil
}
