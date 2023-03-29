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

func (g *ChatGPTPlaylistGenerator) GeneratePlaylist(ctx context.Context, input string) ([]string, error) {
	resp, err := g.openAIClient.CreateChatCompletion(
		context.Background(),
		openai.ChatCompletionRequest{
			Model: openai.GPT3Dot5Turbo,
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleUser,
					Content: fmt.Sprintf("Create a playlist of %s songs", input),
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

	return playlist, nil
}
