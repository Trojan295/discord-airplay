package discord

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/Trojan295/discord-airplay/pkg/bot"
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
	embeds := []*discordgo.MessageEmbed{GeneratePlayingSongEmbed(message)}

	_, err := session.discordSession.ChannelMessageEditComplex(&discordgo.MessageEdit{
		ID:      messageID,
		Channel: channelID,
		Embeds:  &embeds,
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

func (session *DiscordVoiceChatSession) SendAudio(ctx context.Context, opusCh <-chan []byte, positionCallback func(time.Duration)) error {
	if err := session.voiceConnection.Speaking(true); err != nil {
		return fmt.Errorf("while starting to speak: %w", err)
	}

	frameCounter := atomic.Int32{}

outer:
	for {
		select {
		case <-ctx.Done():
			return nil
		case msg, ok := <-opusCh:
			if !ok {
				break outer
			}
			session.voiceConnection.OpusSend <- msg

			go func() {
				if frames := frameCounter.Add(1); frames%50 == 0 {
					positionCallback(time.Duration(frames) * 20 * time.Millisecond)
				}
			}()
		}
	}

	if err := session.voiceConnection.Speaking(false); err != nil {
		return fmt.Errorf("while stopping to speak: %w", err)
	}

	return nil
}
