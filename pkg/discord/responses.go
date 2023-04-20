package discord

import (
	"log"

	"github.com/bwmarrin/discordgo"
	"go.uber.org/zap"
)

func InteractionRespond(logger *zap.Logger, s *discordgo.Session, i *discordgo.Interaction, response *discordgo.InteractionResponse) {
	if err := s.InteractionRespond(i, response); err != nil {
		log.Printf("failed to reponse to interaction: %v", err)
	}
}

func InteractionRespondServerError(logger *zap.Logger, s *discordgo.Session, i *discordgo.Interaction) {
	InteractionRespond(logger, s, i, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: "Airplay has some problems...",
		},
	})
}

func InteractionRespondMessage(logger *zap.Logger, s *discordgo.Session, i *discordgo.Interaction, message string) {
	InteractionRespond(logger, s, i, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: message,
		},
	})
}

func InteractionRespondCommandDisabled(logger *zap.Logger, s *discordgo.Session, i *discordgo.Interaction) {
	InteractionRespond(logger, s, i, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: "🐘 Command is disabled",
		},
	})
}

func FollowupMessageCreate(logger *zap.Logger, s *discordgo.Session, i *discordgo.Interaction, params *discordgo.WebhookParams) {
	if _, err := s.FollowupMessageCreate(i, true, params); err != nil {
		logger.Error("failed to create followup message", zap.Error(err))
	}
}
