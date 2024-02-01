package discord

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/Trojan295/discord-airplay/pkg/bot"
	"github.com/Trojan295/discord-airplay/pkg/codec"
	"github.com/bwmarrin/discordgo"
)

type DiscordVoiceChatSession struct {
	discordSession *discordgo.Session
	guildID        string

	voiceConnection *discordgo.VoiceConnection
}

func (session *DiscordVoiceChatSession) Close() error {
	return session.discordSession.Close()
}

func (session *DiscordVoiceChatSession) SendMessage(channelID, message string) error {
	_, err := session.discordSession.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
		Content: message,
	})
	if err != nil {
		return err
	}

	return nil
}

func (session *DiscordVoiceChatSession) SendPlayMessage(channelID string, message *bot.PlayMessage) (string, error) {
	msg, err := session.discordSession.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
		Embed: GeneratePlayingSongEmbed(message),
	})
	if err != nil {
		return "", err
	}

	return msg.ID, nil
}

func (session *DiscordVoiceChatSession) EditPlayMessage(channelID, messageID string, message *bot.PlayMessage) error {
	_, err := session.discordSession.ChannelMessageEditComplex(&discordgo.MessageEdit{
		ID:      messageID,
		Channel: channelID,
		Embeds:  []*discordgo.MessageEmbed{GeneratePlayingSongEmbed(message)},
	})
	return err
}

func (session *DiscordVoiceChatSession) JoinVoiceChannel(channelID string) error {
	vc, err := session.discordSession.ChannelVoiceJoin(session.guildID, channelID, false, true)
	if err != nil {
		return fmt.Errorf("while joining voice channel: %w", err)
	}

	session.voiceConnection = vc

	return nil
}

func (session *DiscordVoiceChatSession) LeaveVoiceChannel() error {
	if session.voiceConnection == nil {
		return nil
	}

	if err := session.voiceConnection.Disconnect(); err != nil {
		return err
	}

	session.voiceConnection = nil

	return nil
}

func (session *DiscordVoiceChatSession) SendAudio(ctx context.Context, reader io.Reader, positionCallback func(time.Duration)) error {
	if err := session.voiceConnection.Speaking(true); err != nil {
		return fmt.Errorf("while starting to speak: %w", err)
	}

	if err := codec.StreamDCAData(ctx, reader, session.voiceConnection.OpusSend, positionCallback); err != nil {
		return fmt.Errorf("while streaming DCA data: %w", err)
	}

	if err := session.voiceConnection.Speaking(false); err != nil {
		return fmt.Errorf("while stopping to speak: %w", err)
	}

	return nil
}
