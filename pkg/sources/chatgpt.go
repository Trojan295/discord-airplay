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

type PlaylistResponse struct {
	Intro    string
	Playlist []string
}

func (g *ChatGPTPlaylistGenerator) GeneratePlaylist(ctx context.Context, input string) (*PlaylistResponse, error) {
	resp, err := g.openAIClient.CreateChatCompletion(
		context.Background(),
		openai.ChatCompletionRequest{
			Model: openai.GPT3Dot5Turbo,
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleSystem,
					Content: "You are DJ Airyk.",
				},
				{
					Role:    openai.ChatMessageRoleUser,
					Content: "Create for me a playlist.",
				},
				{
					Role:    openai.ChatMessageRoleAssistant,
					Content: "What type of songs should it contain?",
				},
				{
					Role:    openai.ChatMessageRoleUser,
					Content: input,
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
