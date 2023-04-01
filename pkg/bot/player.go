package bot

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"go.uber.org/zap"
)

var ErrNoSongs = errors.New("no songs available")

type Trigger struct {
	Command        string
	VoiceChannelID *string
	TextChannelID  *string
}

type SongMetadata struct {
	Title    string
	URL      string
	Playable bool

	Duration time.Duration
}

type Song interface {
	GetHumanName() string
	GetMetadata() *SongMetadata
	GetDCAData(ctx context.Context) (io.Reader, error)
}

type VoiceChatSession interface {
	Close() error
	SendMessage(channelID string, message string) error
	JoinVoiceChannel(channelID string) error
	LeaveVoiceChannel() error
	SendAudio(ctx context.Context, r io.Reader) error
}

type PlaylistStore interface {
	AppendSong(Song) error
	RemoveSong(int) (Song, error)
	ClearPlaylist() error
	GetSongs() ([]Song, error)
	PopFirstSong() (Song, error)
}

type GuildPlayer struct {
	session VoiceChatSession

	ctx context.Context

	triggerCh      chan Trigger
	voiceChannelID string
	textChannelID  string
	playlistStore  PlaylistStore
	playedSong     Song
	songCtxCancel  context.CancelFunc

	audioBufferSize int

	logger *zap.Logger
}

func NewGuildPlayer(ctx context.Context, session VoiceChatSession, guildID string, playlistStore PlaylistStore) *GuildPlayer {
	return &GuildPlayer{
		ctx:             ctx,
		session:         session,
		triggerCh:       make(chan Trigger),
		playlistStore:   playlistStore,
		playedSong:      nil,
		logger:          zap.NewNop(),
		audioBufferSize: 1024 * 1024, // 1 MiB
	}
}

func (p *GuildPlayer) WithLogger(l *zap.Logger) *GuildPlayer {
	p.logger = l
	return p
}

func (p *GuildPlayer) Close() error {
	p.songCtxCancel()
	return p.session.Close()
}

func (p *GuildPlayer) SendMessage(message string) {
	if err := p.session.SendMessage(p.textChannelID, message); err != nil {
		p.logger.Error("failed to send message", zap.Error(err))
	}
}

func (p *GuildPlayer) AddSong(textChannelID, voiceChannelID *string, s Song) {
	p.playlistStore.AppendSong(s)

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
	p.playlistStore.ClearPlaylist()

	if p.songCtxCancel != nil {
		p.songCtxCancel()
	}
}

func (p *GuildPlayer) RemoveSong(position int) (Song, error) {
	song, err := p.playlistStore.RemoveSong(position)
	if err != nil {
		return nil, fmt.Errorf("while removing song: %w", err)
	}

	return song, nil
}

func (p *GuildPlayer) GetPlaylist() ([]string, error) {
	songs, err := p.playlistStore.GetSongs()
	if err != nil {
		return nil, fmt.Errorf("while getting songs: %w", err)
	}

	playlist := make([]string, len(songs))
	for i, song := range songs {
		playlist[i] = song.GetHumanName()
	}

	return playlist, err
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

				songs, err := p.playlistStore.GetSongs()
				if err != nil {
					p.logger.Error("failed to get songs", zap.Error(err))
					continue
				}

				if len(songs) == 0 {
					continue
				}

				if err := p.playPlaylist(ctx); err != nil {
					p.logger.Error("failed to play playlist", zap.Error(err))
				}
			}
		}
	}
}

func (p *GuildPlayer) playPlaylist(ctx context.Context) error {
	p.logger.Debug("joining voice channel", zap.String("channel", p.voiceChannelID))
	if err := p.session.JoinVoiceChannel(p.voiceChannelID); err != nil {
		return fmt.Errorf("failed to join voice channel: %w", err)
	}

	defer func() {
		p.logger.Debug("leaving voice channel", zap.String("channel", p.voiceChannelID))
		if err := p.session.LeaveVoiceChannel(); err != nil {
			p.logger.Error("failed to leave voice channel", zap.Error(err))
		}
	}()

	for {
		song, err := p.playlistStore.PopFirstSong()
		if err == ErrNoSongs {
			p.logger.Debug("playlist is empty")
			break
		}
		if err != nil {
			return fmt.Errorf("while poping first song: %w", err)
		}

		var songCtx context.Context
		songCtx, p.songCtxCancel = context.WithCancel(ctx)

		metadata := song.GetMetadata()
		message := fmt.Sprintf("▶️ Playing song **%s** - %s", metadata.Title, metadata.URL)

		logger := p.logger.With(zap.String("title", metadata.Title), zap.String("url", metadata.URL))

		if err := p.session.SendMessage(p.textChannelID, message); err != nil {
			return fmt.Errorf("while sending message with song name: %w", err)
		}

		dcaData, err := song.GetDCAData(songCtx)
		if err != nil {
			return fmt.Errorf("while getting DCA data from song %v: %w", song, err)
		}

		audioReader := bufio.NewReaderSize(dcaData, p.audioBufferSize)
		logger.Debug("sending audio stream")
		if err := p.session.SendAudio(songCtx, audioReader); err != nil {
			return fmt.Errorf("while sending audio data: %w", err)
		}

		p.playedSong = nil
		logger.Debug("stopped playing")

		time.Sleep(250 * time.Millisecond)
	}

	return nil
}
