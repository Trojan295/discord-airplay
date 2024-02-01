package discord

import (
	"fmt"
	"strings"
	"time"

	"github.com/Trojan295/discord-airplay/pkg/bot"
	"github.com/Trojan295/discord-airplay/pkg/utils"
	"github.com/bwmarrin/discordgo"
)

var (
	MessageUserNotInVoiceChannel  = "🤷🏽  You are not in a voice channel. Join a voice channel to play a song."
	MessageTooLargePlaylist       = "😨 You cannot request a playlist longer than 20 songs."
	MessageFailedGeneratePlaylist = "😨 Failed to generate playlist."
)

func GenerateAddingSongEmbed(input string, member *discordgo.Member) *discordgo.MessageEmbed {
	return generateAddingSongEmbed(input, "🎵  Adding song to queue...", member)
}

func GenerateAddedSongEmbed(song *bot.Song, member *discordgo.Member) *discordgo.MessageEmbed {
	embed := generateAddingSongEmbed(song.GetHumanName(), "🎵  Added to queue.", member)
	embed.Fields = []*discordgo.MessageEmbedField{
		{
			Name:  "Duration",
			Value: utils.FmtDuration(song.Duration),
		},
	}

	if song.ThumbnailURL != nil {
		embed.Thumbnail = &discordgo.MessageEmbedThumbnail{
			URL: *song.ThumbnailURL,
		}
	}

	return embed
}

func GenerateAskAddPlaylistEmbed(songs []*bot.Song, requestor *discordgo.Member) *discordgo.MessageEmbed {
	title := fmt.Sprintf("👀  The song is part of a playlist, which contains %d songs. What should I do?", len(songs))
	return generateAddingSongEmbed(title, "", requestor)
}

func GenerateFailedToAddSongEmbed(input string, member *discordgo.Member) *discordgo.MessageEmbed {
	return generateAddingSongEmbed(input, "😨  Failed to add song.", member)
}

func GenerateFailedToFindSong(input string, member *discordgo.Member) *discordgo.MessageEmbed {
	return generateAddingSongEmbed(input, "😨 Could not find any playable songs.", member)
}

func GeneratePlayingSongEmbed(message *bot.PlayMessage) *discordgo.MessageEmbed {
	progressBar := generateProgressBar(float64(message.Position)/float64(message.Song.Duration), 20)

	embed := &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("▶️  %s", message.Song.GetHumanName()),
		URL:         message.Song.URL,
		Description: fmt.Sprintf("%s\n%s / %s", progressBar, utils.FmtDuration(message.Position), utils.FmtDuration(message.Song.Duration)),
	}

	if message.Song.ThumbnailURL != nil {
		embed.Thumbnail = &discordgo.MessageEmbedThumbnail{
			URL: *message.Song.ThumbnailURL,
		}
	}

	if message.Song.RequestedBy != nil {
		embed.Footer = &discordgo.MessageEmbedFooter{
			Text: fmt.Sprintf("Requested by %s", *message.Song.RequestedBy),
		}
	}

	return embed
}

func GeneratePlaylistAdded(intro string, songs []*bot.Song, member *discordgo.Member) *discordgo.MessageEmbed {
	descriptionBuilder := strings.Builder{}
	duration := time.Duration(0)

	for _, song := range songs {
		duration += song.Duration
		descriptionBuilder.WriteString(fmt.Sprintf("1.️  %s (%s)\n", song.GetHumanName(), utils.FmtDuration(song.Duration)))
	}

	title := fmt.Sprintf("🎵  %s", intro)

	embed := generateAddingSongEmbed(title, descriptionBuilder.String(), member)
	embed.Fields = []*discordgo.MessageEmbedField{
		{
			Name:  "Duration",
			Value: utils.FmtDuration(duration),
		},
	}

	return embed
}

func generateAddingSongEmbed(title, description string, requestor *discordgo.Member) *discordgo.MessageEmbed {
	embed := &discordgo.MessageEmbed{
		Title:       title,
		Description: description,
		Footer: &discordgo.MessageEmbedFooter{
			Text: fmt.Sprintf("Requested by %s", getMemberName(requestor)),
		},
	}

	return embed
}

func generateProgressBar(progress float64, length int) string {
	played := int(progress * float64(length))

	progressBar := ""
	for i := 0; i < played; i++ {
		progressBar += "▬"
	}
	progressBar += "🔘"
	for i := played; i < length; i++ {
		progressBar += "▬"
	}

	return progressBar
}
