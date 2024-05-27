package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/sashabaranov/go-openai"
	"golang.org/x/exp/slog"
)

const AssistantID = "asst_XaJbt5qXnInquWa1K6wNUYUT"

type ChatGPTPlaylistGenerator struct {
	Logger *slog.Logger

	openAIClient *openai.Client
	model        string
	assistantID  string
}

func NewChatGPTPlaylistGenerator(token string) *ChatGPTPlaylistGenerator {
	config := openai.DefaultConfig(token)
	config.AssistantVersion = "v2"
	client := openai.NewClientWithConfig(config)
	return &ChatGPTPlaylistGenerator{
		Logger:       slog.Default(),
		openAIClient: client,
		model:        openai.GPT4o,
		assistantID:  AssistantID,
	}
}

type PlaylistParams struct {
	Description string `json:"description"`
	Length      int    `json:"length"`
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

	messageContent, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("while marshaling playlist params: %w", err)
	}

	run, err := g.openAIClient.CreateThreadAndRun(ctx, openai.CreateThreadAndRunRequest{
		RunRequest: openai.RunRequest{
			Model:       g.model,
			AssistantID: g.assistantID,
		},
		Thread: openai.ThreadRequest{
			Messages: []openai.ThreadMessage{
				{
					Role:    openai.ThreadMessageRoleUser,
					Content: string(messageContent),
				},
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("while creating thread and running: %w", err)
	}

	runWaitCtx, cancel := context.WithTimeout(ctx, time.Duration(15*time.Second))
	defer cancel()

	runCh := make(chan *openai.Run)
	go func(ctx context.Context) {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				run, err := g.openAIClient.RetrieveRun(ctx, run.ThreadID, run.ID)
				if err != nil {
					time.Sleep(1 * time.Second)
					continue
				}

				if run.Status == openai.RunStatusCompleted {
					runCh <- &run
					return
				}

				time.Sleep(1 * time.Second)
			}
		}
	}(runWaitCtx)

	select {
	case <-runWaitCtx.Done():
		return nil, fmt.Errorf("timeout while waiting for run to complete")

	case run := <-runCh:
		messageList, err := g.openAIClient.ListMessage(ctx, run.ThreadID, nil, nil, nil, nil)
		if err != nil {
			return nil, fmt.Errorf("while listing messages: %w", err)
		}

		if len(messageList.Messages) != 2 {
			return nil, fmt.Errorf("unexpected number of messages: %d", len(messageList.Messages))
		}

		if len(messageList.Messages[0].Content) != 1 {
			return nil, fmt.Errorf("unexpected number of message content: %d", len(messageList.Messages[0].Content))
		}

		response := &PlaylistResponse{}
		responseText := messageList.Messages[0].Content[0].Text.Value
		if err := json.Unmarshal([]byte(responseText), response); err != nil {
			return nil, fmt.Errorf("while unmarshaling response: %w", err)
		}

		return response, nil
	}
}
