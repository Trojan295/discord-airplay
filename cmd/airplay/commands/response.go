package commands

import (
	"log"

	"github.com/bwmarrin/discordgo"
)

func InteractionRespondError(s *discordgo.Session, i *discordgo.Interaction) {
	if err := s.InteractionRespond(i, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: "airplay has some problems...",
		},
	}); err != nil {
		log.Printf("failed to reponse to interaction: %v", err)
	}
}

func InteractionRespondMessage(s *discordgo.Session, i *discordgo.Interaction, message string) {
	InteractionRespond(s, i, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: message,
		},
	})
}

func InteractionRespond(s *discordgo.Session, i *discordgo.Interaction, response *discordgo.InteractionResponse) {
	if err := s.InteractionRespond(i, response); err != nil {
		log.Printf("failed to reponse to interaction: %v", err)
	}
}

func InteractionRespondCommandDisabled(s *discordgo.Session, i *discordgo.Interaction) {
	if err := s.InteractionRespond(i, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: "🐘 Command is disabled",
		},
	}); err != nil {
		log.Printf("failed to reponse to interaction: %v", err)
	}
}

func FollowupMessageCreate(s *discordgo.Session, i *discordgo.Interaction, params *discordgo.WebhookParams) {
	if _, err := s.FollowupMessageCreate(i, true, params); err != nil {
		log.Printf("failed to create followup message: %v", err)
	}
}
