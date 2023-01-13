package bot

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/Trojan295/discord-airplay/pkg/codec"
	"github.com/bwmarrin/discordgo"
)

type Trigger struct {
	VoiceChannelID string
	TextChannelID  string
}

type SongMetadata struct {
	Title    string
	URL      string
	Playable bool
}

type Song interface {
	GetHumanName() string
	GetMetadata(ctx context.Context) (*SongMetadata, error)
	GetDCAData(ctx context.Context) ([]byte, error)
}

type GuildPlayer struct {
	sess    *discordgo.Session
	guildID string

	triggerCh chan Trigger

	mutex          sync.Mutex
	voiceChannelID string
	textChannelID  string
	playlist       []Song

	songCtxCancel context.CancelFunc
}

func NewGuildPlayer(sess *discordgo.Session, guildID string) *GuildPlayer {
	return &GuildPlayer{
		sess:      sess,
		guildID:   guildID,
		triggerCh: make(chan Trigger),
		playlist:  []Song{},
		mutex:     sync.Mutex{},
	}
}

func (p *GuildPlayer) AddSong(textChannelID, voiceChannelID string, s Song) {
	p.mutex.Lock()
	p.playlist = append(p.playlist, s)
	p.mutex.Unlock()

	// prefetch the DCA data
	go func(s Song) {
		if _, err := s.GetDCAData(context.TODO()); err != nil {
			log.Printf("failed to get DCA data for song %s: %v", s.GetHumanName(), err)
		}
	}(s)

	go func() {
		p.triggerCh <- Trigger{VoiceChannelID: voiceChannelID, TextChannelID: textChannelID}
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

func (p *GuildPlayer) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case trigger := <-p.triggerCh:
			p.voiceChannelID = trigger.VoiceChannelID
			p.textChannelID = trigger.TextChannelID
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

func (p *GuildPlayer) playPlaylist(ctx context.Context) error {
	vc, err := p.sess.ChannelVoiceJoin(p.guildID, p.voiceChannelID, false, true)
	if err != nil {
		return fmt.Errorf("while joining voice channel: %w", err)
	}

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

		opus, err := codec.ConvertDCAtoOpus(bytes.NewBuffer(dcaData))
		if err != nil {
			return fmt.Errorf("while converting DCA data to Opus: %w", err)
		}

		var message string

		if metadata, err := song.GetMetadata(ctx); err != nil {
			message = fmt.Sprintf("Playing song **%s**", song.GetHumanName())
		} else {
			message = fmt.Sprintf("Playing song **%s** - %s", metadata.Title, metadata.URL)
		}

		if _, err := p.sess.ChannelMessageSendComplex(p.textChannelID, &discordgo.MessageSend{
			Content: message,
		}); err != nil {
			log.Printf("failed to send playing song message: %v", err)
		}

		if err := vc.Speaking(true); err != nil {
			return fmt.Errorf("while starting to speak: %w", err)
		}

		for _, buff := range opus {
			select {
			case vc.OpusSend <- buff:
				continue
			case <-songCtx.Done():
				break
			}
		}

		if err := vc.Speaking(false); err != nil {
			return fmt.Errorf("while sstopping to speak: %w", err)
		}

		time.Sleep(250 * time.Millisecond)
	}

	return nil
}
