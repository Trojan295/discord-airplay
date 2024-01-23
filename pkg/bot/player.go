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

type Song struct {
	Type string

	Title    string
	URL      string
	Playable bool

	Duration      time.Duration
	StartPosition time.Duration
}

func (s *Song) GetHumanName() string {
	if s.Title != "" {
		return s.Title
	}

	return s.URL
}

type PlayMessage struct {
	Song     *Song
	Position time.Duration
}

type VoiceChatSession interface {
	Close() error
	SendMessage(channelID, message string) error
	SendPlayMessage(channelID string, message *PlayMessage) (string, error)
	EditPlayMessage(channelID, messageID string, message *PlayMessage) error
	JoinVoiceChannel(channelID string) error
	LeaveVoiceChannel() error
	SendAudio(ctx context.Context, r io.Reader, positionCallback func(time.Duration)) error
}

type DCADataGetter func(ctx context.Context, song *Song) (io.Reader, error)

type PlayedSong struct {
	Song
	Position time.Duration
}

type GuildPlayerState interface {
	PrependSong(*Song) error
	AppendSong(*Song) error
	RemoveSong(int) (*Song, error)
	ClearPlaylist() error
	GetSongs() ([]*Song, error)
	PopFirstSong() (*Song, error)

	SetVoiceChannel(string) error
	GetVoiceChannel() (string, error)

	SetTextChannel(string) error
	GetTextChannel() (string, error)

	GetCurrentSong() (*PlayedSong, error)
	SetCurrentSong(*PlayedSong) error
}

type GuildPlayer struct {
	session VoiceChatSession

	state GuildPlayerState

	ctx context.Context

	triggerCh     chan Trigger
	songCtxCancel context.CancelFunc

	dCADataGetter   DCADataGetter
	audioBufferSize int

	logger *zap.Logger
}

var (
	ErrRemoveInvalidPosition = errors.New("invalid position")
)

func NewGuildPlayer(ctx context.Context, session VoiceChatSession, guildID string, state GuildPlayerState, dCADataGetter DCADataGetter) *GuildPlayer {
	return &GuildPlayer{
		ctx:             ctx,
		state:           state,
		session:         session,
		triggerCh:       make(chan Trigger),
		logger:          zap.NewNop(),
		dCADataGetter:   dCADataGetter,
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
	channel, err := p.state.GetTextChannel()
	if err != nil {
		p.logger.Error("failed to get text channel", zap.Error(err))
		return
	}

	if err := p.session.SendMessage(channel, message); err != nil {
		p.logger.Error("failed to send message", zap.Error(err))
	}
}

func (p *GuildPlayer) AddSong(textChannelID, voiceChannelID *string, s *Song) error {
	if err := p.state.AppendSong(s); err != nil {
		return fmt.Errorf("while appending song: %w", err)
	}

	go func() {
		p.triggerCh <- Trigger{
			Command:        "play",
			VoiceChannelID: voiceChannelID,
			TextChannelID:  textChannelID,
		}
	}()

	return nil
}

func (p *GuildPlayer) SkipSong() {
	if p.songCtxCancel != nil {
		p.songCtxCancel()
	}
}

func (p *GuildPlayer) Stop() error {
	if err := p.state.ClearPlaylist(); err != nil {
		return fmt.Errorf("while clearing playlist: %w", err)
	}

	if p.songCtxCancel != nil {
		p.songCtxCancel()
	}

	return nil
}

func (p *GuildPlayer) RemoveSong(position int) (*Song, error) {
	song, err := p.state.RemoveSong(position)
	if err != nil {
		return nil, fmt.Errorf("while removing song: %w", err)
	}

	return song, nil
}

func (p *GuildPlayer) GetPlaylist() ([]string, error) {
	songs, err := p.state.GetSongs()
	if err != nil {
		return nil, fmt.Errorf("while getting songs: %w", err)
	}

	playlist := make([]string, len(songs))
	for i, song := range songs {
		playlist[i] = song.GetHumanName()
	}

	return playlist, err
}

func (p *GuildPlayer) GetPlayedSong() (*PlayedSong, error) {
	return p.state.GetCurrentSong()
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
	currentSong, err := p.state.GetCurrentSong()
	if err != nil {
		p.logger.Info("failed to get current song", zap.Error(err))
	} else if currentSong != nil {
		currentSong.StartPosition += currentSong.Position

		if err := p.state.PrependSong(&currentSong.Song); err != nil {
			p.logger.Info("failed to prepend current song in the playlist", zap.Error(err))
		}
	}

	songs, err := p.state.GetSongs()
	if err != nil {
		return fmt.Errorf("while getting songs: %w", err)
	}

	if len(songs) > 0 {
		voiceChannel, err := p.state.GetVoiceChannel()
		if err != nil {
			return fmt.Errorf("while getting voice channel: %w", err)
		}
		textChannel, err := p.state.GetTextChannel()
		if err != nil {
			return fmt.Errorf("while getting text channel: %w", err)
		}

		go func() {
			p.triggerCh <- Trigger{
				Command:        "play",
				VoiceChannelID: &voiceChannel,
				TextChannelID:  &textChannel,
			}
		}()
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case trigger := <-p.triggerCh:
			switch trigger.Command {
			case "play":
				if trigger.TextChannelID != nil {
					if err := p.state.SetTextChannel(*trigger.TextChannelID); err != nil {
						return fmt.Errorf("while setting text channel: %w", err)
					}
				}
				if trigger.VoiceChannelID != nil {
					if err := p.state.SetVoiceChannel(*trigger.VoiceChannelID); err != nil {
						return fmt.Errorf("while setting voice channel: %w", err)
					}
				}

				songs, err := p.state.GetSongs()
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
	voiceChannel, err := p.state.GetVoiceChannel()
	if err != nil {
		return fmt.Errorf("while getting voice channel: %w", err)
	}

	textChannel, err := p.state.GetTextChannel()
	if err != nil {
		return fmt.Errorf("while getting text channel: %w", err)
	}

	p.logger.Debug("joining voice channel", zap.String("channel", voiceChannel))
	if err := p.session.JoinVoiceChannel(voiceChannel); err != nil {
		return fmt.Errorf("failed to join voice channel: %w", err)
	}

	defer func() {
		p.logger.Debug("leaving voice channel", zap.String("channel", voiceChannel))
		if err := p.session.LeaveVoiceChannel(); err != nil {
			p.logger.Error("failed to leave voice channel", zap.Error(err))
		}
	}()

	for {
		song, err := p.state.PopFirstSong()
		if err == ErrNoSongs {
			p.logger.Debug("playlist is empty")
			break
		}
		if err != nil {
			return fmt.Errorf("while poping first song: %w", err)
		}

		if err := p.state.SetCurrentSong(&PlayedSong{Song: *song}); err != nil {
			return fmt.Errorf("while setting current song: %w", err)
		}

		var songCtx context.Context
		songCtx, p.songCtxCancel = context.WithCancel(ctx)

		logger := p.logger.With(zap.String("title", song.Title), zap.String("url", song.URL))

		playMsgID, err := p.session.SendPlayMessage(textChannel, &PlayMessage{
			Song: song,
		})
		if err != nil {
			return fmt.Errorf("while sending message with song name: %w", err)
		}

		dcaData, err := p.dCADataGetter(songCtx, song)
		if err != nil {
			return fmt.Errorf("while getting DCA data from song %v: %w", song, err)
		}

		audioReader := bufio.NewReaderSize(dcaData, p.audioBufferSize)
		logger.Debug("sending audio stream")
		if err := p.session.SendAudio(songCtx, audioReader, func(d time.Duration) {
			if err := p.state.SetCurrentSong(&PlayedSong{Song: *song, Position: d}); err != nil {
				logger.Error("failed to set current song position", zap.Error(err))
			}
			if err := p.session.EditPlayMessage(textChannel, playMsgID, &PlayMessage{Song: song, Position: d}); err != nil {
				logger.Error("failed to edit message", zap.Error(err))
			}

		}); err != nil {
			return fmt.Errorf("while sending audio data: %w", err)
		}

		if err := p.state.SetCurrentSong(nil); err != nil {
			return fmt.Errorf("while setting current song: %w", err)
		}
		logger.Debug("stopped playing")

		time.Sleep(250 * time.Millisecond)
	}

	return nil
}
