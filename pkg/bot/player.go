package bot

import (
	"context"
	"fmt"
	"io"
	"log"
	"sync"
	"time"

	"github.com/Trojan295/discord-airplay/pkg/codec"
	"github.com/bwmarrin/discordgo"
)

type Trigger struct {
	Command        string
	VoiceChannelID *string
	TextChannelID  *string
}

type SongMetadata struct {
	Title    string
	URL      string
	Playable bool
}

type Song interface {
	GetHumanName() string
	GetMetadata() *SongMetadata
	GetDCAData(ctx context.Context) (io.Reader, error)
}

type ASRService interface {
	FeedOpusData(opusData []byte) error
}

type GuildPlayer struct {
	sess    *discordgo.Session
	guildID string

	ctx context.Context

	triggerCh       chan Trigger
	mutex           sync.Mutex
	voiceChannelID  string
	textChannelID   string
	voiceConnection *discordgo.VoiceConnection
	playlist        []Song
	playedSong      Song

	songCtxCancel context.CancelFunc
}

func NewGuildPlayer(ctx context.Context, sess *discordgo.Session, guildID string) *GuildPlayer {
	return &GuildPlayer{
		ctx:        ctx,
		sess:       sess,
		guildID:    guildID,
		triggerCh:  make(chan Trigger),
		playlist:   []Song{},
		playedSong: nil,
		mutex:      sync.Mutex{},
	}
}

func (p *GuildPlayer) SendMessage(message string) {
	if p.textChannelID != "" {
		if _, err := p.sess.ChannelMessageSendComplex(p.textChannelID, &discordgo.MessageSend{
			Content: message,
		}); err != nil {
			log.Printf("failed to send message to guild %s: %v", p.guildID, err.Error())
		}
	}
}

func (p *GuildPlayer) AddSong(textChannelID, voiceChannelID *string, s Song) {
	p.mutex.Lock()
	p.playlist = append(p.playlist, s)
	p.mutex.Unlock()

	go func() {
		p.triggerCh <- Trigger{
			Command:        "play",
			VoiceChannelID: voiceChannelID,
			TextChannelID:  textChannelID,
		}
	}()
}

func (p *GuildPlayer) SkipSong() {
	if p.songCtxCancel != nil {
		p.songCtxCancel()
	}
}

func (p *GuildPlayer) Stop() {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	p.playlist = make([]Song, 0)

	if p.songCtxCancel != nil {
		p.songCtxCancel()
	}
}

func (p *GuildPlayer) RemoveSong(position int) (Song, error) {
	index := position - 1

	p.mutex.Lock()
	defer p.mutex.Unlock()

	if index < 0 || index > len(p.playlist)-1 {
		return nil, fmt.Errorf("wrong position")
	}

	song := p.playlist[index]
	p.playlist = append(p.playlist[:index], p.playlist[index+1:]...)

	return song, nil
}

func (p *GuildPlayer) GetPlaylist() []string {
	playlist := []string{}

	p.mutex.Lock()
	defer p.mutex.Unlock()

	for _, song := range p.playlist {
		playlist = append(playlist, song.GetHumanName())
	}

	return playlist
}

func (p *GuildPlayer) GetPlayedSong() Song {
	return p.playedSong
}

func (p *GuildPlayer) JoinVoiceChannel(channelID, textChannelID string) {
	p.triggerCh <- Trigger{
		Command:        "join",
		VoiceChannelID: &channelID,
		TextChannelID:  &textChannelID,
	}
}

func (p *GuildPlayer) LeaveVoiceChannel() {
	p.triggerCh <- Trigger{
		Command: "leave",
	}
}

func (p *GuildPlayer) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case trigger := <-p.triggerCh:
			switch trigger.Command {
			case "play":
				if trigger.TextChannelID != nil {
					p.textChannelID = *trigger.TextChannelID
				}
				if trigger.VoiceChannelID != nil {
					p.voiceChannelID = *trigger.VoiceChannelID
				}

				p.mutex.Lock()
				playlistLen := len(p.playlist)
				p.mutex.Unlock()

				if playlistLen == 0 {
					continue
				}

				if err := p.playPlaylist(ctx); err != nil {
					log.Printf("failed to play playlist: %v", err)
				}
			}
		}
	}
}

func (p *GuildPlayer) playPlaylist(ctx context.Context) error {
	vc, err := p.sess.ChannelVoiceJoin(p.guildID, p.voiceChannelID, false, true)
	if err != nil {
		return fmt.Errorf("while joining voice channel: %w", err)
	}

	p.voiceConnection = vc

	defer func() {
		if err := vc.Disconnect(); err != nil {
			log.Printf("failed to disconnect from voice channel: %v", err)
		}
	}()

	for {
		p.mutex.Lock()
		playlistLen := len(p.playlist)
		p.mutex.Unlock()

		if playlistLen == 0 {
			break
		}

		p.mutex.Lock()
		song := p.playlist[0]
		p.playlist = p.playlist[1:]

		var songCtx context.Context
		songCtx, p.songCtxCancel = context.WithCancel(ctx)

		p.mutex.Unlock()

		dcaData, err := song.GetDCAData(songCtx)
		if err != nil {
			return fmt.Errorf("while getting DCA data from song %v: %w", song, err)
		}

		metadata := song.GetMetadata()
		message := fmt.Sprintf("▶️ Playing song **%s** - %s", metadata.Title, metadata.URL)
		if _, err := p.sess.ChannelMessageSendComplex(p.textChannelID, &discordgo.MessageSend{
			Content: message,
		}); err != nil {
			log.Printf("failed to send playing song message: %v", err)
		}

		if err := p.voiceConnection.Speaking(true); err != nil {
			return fmt.Errorf("while starting to speak: %w", err)
		}

		p.playedSong = song

		if err := codec.StreamDCAData(songCtx, dcaData, p.voiceConnection.OpusSend); err != nil {
			return fmt.Errorf("while streaming DCA data: %w", err)
		}

		p.playedSong = nil

		if err := p.voiceConnection.Speaking(false); err != nil {
			return fmt.Errorf("while sstopping to speak: %w", err)
		}

		time.Sleep(250 * time.Millisecond)
	}

	return nil
}
