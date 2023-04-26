package discord

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Trojan295/discord-airplay/pkg/bot"
	"github.com/Trojan295/discord-airplay/pkg/config"
	"github.com/Trojan295/discord-airplay/pkg/sources"
	"github.com/bwmarrin/discordgo"
	"go.uber.org/zap"
)

type GuildID string

type SongLookuper interface {
	LookupSongs(ctx context.Context, input string) ([]*bot.Song, error)
}

type PlaylistGenerator interface {
	GeneratePlaylist(ctx context.Context, params *sources.PlaylistParams) (*sources.PlaylistResponse, error)
}

type InteractionStorage interface {
	SaveSongList(channelID string, list []*bot.Song)
	GetSongList(channelID string) []*bot.Song
	DeleteSongList(channelID string)
}

type InteractionHandler struct {
	ctx          context.Context
	discordToken string

	guildPlayers map[GuildID]*bot.GuildPlayer

	playlistGenerator PlaylistGenerator
	songLookuper      SongLookuper
	storage           InteractionStorage

	cfg *config.Config // TODO: replace with a playlist store, which supports multiple guilds

	logger *zap.Logger
}

func NewInteractionHandler(ctx context.Context, discordToken string, songLookuper SongLookuper, playlistGenerator PlaylistGenerator, storage InteractionStorage, cfg *config.Config) *InteractionHandler {
	handler := &InteractionHandler{
		ctx:               ctx,
		discordToken:      discordToken,
		guildPlayers:      make(map[GuildID]*bot.GuildPlayer),
		playlistGenerator: playlistGenerator,
		songLookuper:      songLookuper,
		storage:           storage,
		cfg:               cfg,
		logger:            zap.NewNop(),
	}

	return handler
}

func (handler *InteractionHandler) WithLogger(l *zap.Logger) *InteractionHandler {
	handler.logger = l
	return handler
}

func (handler *InteractionHandler) Ready(s *discordgo.Session, event *discordgo.Ready) {
	if err := s.UpdateGameStatus(0, "🕺💃 /air"); err != nil {
		handler.logger.Error("failed to update game status", zap.Error(err))
	}
}

func (handler *InteractionHandler) GuildCreate(s *discordgo.Session, event *discordgo.GuildCreate) {
	if event.Guild.Unavailable {
		return
	}

	player := handler.setupGuildPlayer(GuildID(event.Guild.ID))
	handler.guildPlayers[GuildID(event.Guild.ID)] = player

	handler.logger.Info("connected to guild", zap.String("guildID", event.Guild.ID))

	go func() {
		if err := player.Run(handler.ctx); err != nil {
			handler.logger.Error("error occured, when player was running", zap.Error(err))
		}
	}()
}

func (handler *InteractionHandler) GuildDelete(s *discordgo.Session, event *discordgo.GuildDelete) {
	guildID := GuildID(event.Guild.ID)

	player := handler.getGuildPlayer(guildID)
	player.Close()

	delete(handler.guildPlayers, guildID)
}

func (handler *InteractionHandler) PlaySong(s *discordgo.Session, ic *discordgo.InteractionCreate, opt *discordgo.ApplicationCommandInteractionDataOption) {
	logger := handler.logger.With(zap.String("guildID", ic.GuildID))

	g, err := s.State.Guild(ic.GuildID)
	if err != nil {
		logger.Info("failed to get guild", zap.Error(err))
		InteractionRespondServerError(handler.logger, s, ic.Interaction)
		return
	}

	player := handler.getGuildPlayer(GuildID(g.ID))

	optionMap := make(map[string]*discordgo.ApplicationCommandInteractionDataOption, len(opt.Options))
	for _, opt := range opt.Options {
		optionMap[opt.Name] = opt
	}

	input := optionMap["input"].StringValue()

	for _, vs := range g.VoiceStates {
		if vs.UserID == ic.Member.User.ID {
			InteractionRespond(handler.logger, s, ic.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "⏳ Adding song...",
				},
			})

			go func() {
				songs, err := handler.songLookuper.LookupSongs(handler.ctx, input)
				if err != nil {
					logger.Info("failed to lookup song metadata", zap.Error(err), zap.String("input", input))
					FollowupMessageCreate(handler.logger, s, ic.Interaction, &discordgo.WebhookParams{
						Content: "😨 Failed to add song",
					})
					return
				}

				if len(songs) == 0 {
					FollowupMessageCreate(handler.logger, s, ic.Interaction, &discordgo.WebhookParams{
						Content: "😨 Could not find any playable songs",
					})
					return
				}

				if len(songs) == 1 {
					song := songs[0]
					metadata := song

					if err := player.AddSong(&ic.ChannelID, &vs.ChannelID, song); err != nil {
						logger.Info("failed to add song", zap.Error(err), zap.String("input", input))
						FollowupMessageCreate(handler.logger, s, ic.Interaction, &discordgo.WebhookParams{
							Content: "😨 Failed to add song",
						})
						return
					}

					FollowupMessageCreate(handler.logger, s, ic.Interaction, &discordgo.WebhookParams{
						Content: fmt.Sprintf("➕ Added **%s** (%s) - %s to playlist", metadata.Title, metadata.Duration, metadata.URL),
					})
				} else {
					handler.storage.SaveSongList(ic.ChannelID, songs)

					FollowupMessageCreate(handler.logger, s, ic.Interaction, &discordgo.WebhookParams{
						Content: fmt.Sprintf("👀 The song is part of a playlist, which contains %d songs. What should I do?", len(songs)),
						Components: []discordgo.MessageComponent{
							discordgo.ActionsRow{
								Components: []discordgo.MessageComponent{
									discordgo.SelectMenu{
										CustomID: "add_song_playlist",
										Options: []discordgo.SelectMenuOption{
											{Label: "Add song", Value: "song", Emoji: discordgo.ComponentEmoji{Name: "🎵"}},
											{Label: "Add whole playlist", Value: "playlist", Emoji: discordgo.ComponentEmoji{Name: "🎶"}},
										},
									},
								},
							},
						},
					})
				}
			}()

			return
		}
	}

	InteractionRespondMessage(handler.logger, s, ic.Interaction, "🤷🏽 You are not in a voice channel. Join a voice channel to play a song.")
}

func (handler *InteractionHandler) CreatePlaylist(s *discordgo.Session, ic *discordgo.InteractionCreate, opt *discordgo.ApplicationCommandInteractionDataOption) {
	logger := handler.logger.With(zap.String("guildID", ic.GuildID))

	g, err := s.State.Guild(ic.GuildID)
	if err != nil {
		logger.Info("failed to get guild", zap.Error(err))
		InteractionRespondServerError(handler.logger, s, ic.Interaction)
		return
	}

	player := handler.getGuildPlayer(GuildID(g.ID))

	optionMap := make(map[string]*discordgo.ApplicationCommandInteractionDataOption, len(opt.Options))
	for _, opt := range opt.Options {
		optionMap[opt.Name] = opt
	}

	description := optionMap["description"].StringValue()

	var length int64 = 10
	lengthOpt, ok := optionMap["length"]
	if ok {
		length = lengthOpt.IntValue()
	}

	var voiceState *discordgo.VoiceState

	for _, vs := range g.VoiceStates {
		if vs.UserID == ic.Member.User.ID {
			voiceState = vs
			break
		}
	}

	if voiceState == nil {
		InteractionRespondMessage(handler.logger, s, ic.Interaction, "🤷🏽 You are not in a voice channel. Join a voice channel to play a song.")
		return
	}

	go func() {
		playlist, err := handler.playlistGenerator.GeneratePlaylist(handler.ctx, &sources.PlaylistParams{
			Description: description,
			Length:      int(length),
		})
		if err != nil {
			logger.Info("failed to generate playlist", zap.Error(err))
			FollowupMessageCreate(handler.logger, s, ic.Interaction, &discordgo.WebhookParams{
				Content: "😨 Failed to generate playlist",
			})
			return
		}

		logger.Debug("generated playlist", zap.Any("songs", playlist.Playlist))

		resposeMessage := strings.Builder{}

		for _, input := range playlist.Playlist {
			songs, err := handler.songLookuper.LookupSongs(handler.ctx, input)
			if err != nil {
				logger.Info("failed to lookup song metadata", zap.Error(err), zap.String("input", input))
				continue
			}

			if len(songs) == 0 {
				continue
			}

			song := songs[0]
			if err := player.AddSong(&ic.ChannelID, &voiceState.ChannelID, song); err != nil {
				logger.Info("failed to add song", zap.Error(err), zap.String("input", input))
				continue
			}

			resposeMessage.WriteString(fmt.Sprintf("- %s (%s)\n", song.Title, song.Duration))
		}

		FollowupMessageCreate(logger, s, ic.Interaction, &discordgo.WebhookParams{
			Content: playlist.Intro,
			Embeds: []*discordgo.MessageEmbed{
				{Title: "Songs:", Description: resposeMessage.String()},
			},
		})
	}()

	InteractionRespond(logger, s, ic.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: "⏳ Generating playlist...",
		},
	})
}

func (handler *InteractionHandler) AddSongOrPlaylist(s *discordgo.Session, ic *discordgo.InteractionCreate) {
	values := ic.MessageComponentData().Values
	if len(values) == 0 {
		InteractionRespondMessage(handler.logger, s, ic.Interaction, "😨 Something went wrong...")
		return
	}

	g, err := s.State.Guild(ic.GuildID)
	if err != nil {
		handler.logger.Info("failed to get guild", zap.Error(err))
		InteractionRespondServerError(handler.logger, s, ic.Interaction)
		return
	}

	value := values[0]
	songs := handler.storage.GetSongList(ic.ChannelID)
	if len(songs) == 0 {
		InteractionRespondMessage(handler.logger, s, ic.Interaction, "Interaction was already selected")
		return
	}

	player := handler.getGuildPlayer(GuildID(g.ID))

	var voiceChannelID *string = nil

	for _, vs := range g.VoiceStates {
		if vs.UserID == ic.Member.User.ID {
			voiceChannelID = &vs.ChannelID
			break
		}
	}

	if voiceChannelID == nil {
		InteractionRespondMessage(handler.logger, s, ic.Interaction, "🤷🏽 You are not in a voice channel. Join a voice channel to play a song.")
		return
	}

	switch value {
	case "playlist":
		for _, song := range songs {
			if err := player.AddSong(&ic.Message.ChannelID, voiceChannelID, song); err != nil {
				handler.logger.Info("failed to add song", zap.Error(err), zap.String("input", song.URL))
			}
		}
		InteractionRespondMessage(handler.logger, s, ic.Interaction, fmt.Sprintf("➕ Added %d songs to playlist", len(songs)))
	default:
		song := songs[0]
		if err := player.AddSong(&ic.Message.ChannelID, voiceChannelID, song); err != nil {
			handler.logger.Info("failed to add song", zap.Error(err), zap.String("input", song.URL))
			InteractionRespondMessage(handler.logger, s, ic.Interaction, "😨 Failed to add song")
		} else {
			InteractionRespondMessage(handler.logger, s, ic.Interaction, fmt.Sprintf("➕ Added **%s** - %s to playlist", song.Title, song.URL))
		}
	}

	handler.storage.DeleteSongList(ic.ChannelID)
}

func (handler *InteractionHandler) StopPlaying(s *discordgo.Session, ic *discordgo.InteractionCreate, acido *discordgo.ApplicationCommandInteractionDataOption) {
	g, err := s.State.Guild(ic.GuildID)
	if err != nil {
		handler.logger.Info("failed to get guild", zap.Error(err))
		InteractionRespondServerError(handler.logger, s, ic.Interaction)
		return
	}

	player := handler.getGuildPlayer(GuildID(g.ID))
	if err := player.Stop(); err != nil {
		handler.logger.Info("failed to stop playing", zap.Error(err))
		InteractionRespondServerError(handler.logger, s, ic.Interaction)
		return
	}

	InteractionRespondMessage(handler.logger, s, ic.Interaction, "⏹️ Stopped playing!")
}

func (handler *InteractionHandler) SkipSong(s *discordgo.Session, ic *discordgo.InteractionCreate, acido *discordgo.ApplicationCommandInteractionDataOption) {
	g, err := s.State.Guild(ic.GuildID)
	if err != nil {
		handler.logger.Info("failed to get guild", zap.Error(err))
		InteractionRespondServerError(handler.logger, s, ic.Interaction)
		return
	}

	player := handler.getGuildPlayer(GuildID(g.ID))
	player.SkipSong()

	InteractionRespondMessage(handler.logger, s, ic.Interaction, "⏭️ Skipped song")
}

func (handler *InteractionHandler) ListPlaylist(s *discordgo.Session, ic *discordgo.InteractionCreate, acido *discordgo.ApplicationCommandInteractionDataOption) {
	g, err := s.State.Guild(ic.GuildID)
	if err != nil {
		handler.logger.Info("failed to get guild", zap.Error(err))
		InteractionRespondServerError(handler.logger, s, ic.Interaction)
		return
	}

	player := handler.getGuildPlayer(GuildID(g.ID))
	playlist, err := player.GetPlaylist()
	if err != nil {
		handler.logger.Error("failed to get playlist", zap.Error(err))
		return
	}

	if len(playlist) == 0 {
		InteractionRespondMessage(handler.logger, s, ic.Interaction, "🫙 Playlist is empty")
	} else {
		builder := strings.Builder{}

		for idx, song := range playlist {
			line := fmt.Sprintf("%d. %s\n", idx+1, song)

			if len(line)+builder.Len() > 4000 {
				builder.WriteString("...")
				break
			}

			builder.WriteString(fmt.Sprintf("%d. %s\n", idx+1, song))
		}

		message := strings.TrimSpace(builder.String())

		InteractionRespond(handler.logger, s, ic.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Embeds: []*discordgo.MessageEmbed{
					{Title: "Playlist:", Description: message},
				},
			},
		})
	}
}

func (handler *InteractionHandler) RemoveSong(s *discordgo.Session, ic *discordgo.InteractionCreate, opt *discordgo.ApplicationCommandInteractionDataOption) {
	g, err := s.State.Guild(ic.GuildID)
	if err != nil {
		handler.logger.Info("failed to get guild", zap.Error(err))
		InteractionRespondServerError(handler.logger, s, ic.Interaction)
		return
	}

	player := handler.getGuildPlayer(GuildID(g.ID))

	optionMap := make(map[string]*discordgo.ApplicationCommandInteractionDataOption, len(opt.Options))
	for _, opt := range opt.Options {
		optionMap[opt.Name] = opt
	}

	position := optionMap["position"].IntValue()

	song, err := player.RemoveSong(int(position))
	if err != nil {
		if errors.Is(err, bot.ErrRemoveInvalidPosition) {
			InteractionRespondMessage(handler.logger, s, ic.Interaction, "🤷🏽 Invalid position")
			return
		}

		handler.logger.Error("failed to remove song", zap.Error(err))
		InteractionRespondServerError(handler.logger, s, ic.Interaction)
		return
	}

	InteractionRespondMessage(handler.logger, s, ic.Interaction, fmt.Sprintf("🗑️ Removed song **%v** from playlist", song.GetHumanName()))
}

func (handler *InteractionHandler) GetPlayingSong(s *discordgo.Session, ic *discordgo.InteractionCreate, opt *discordgo.ApplicationCommandInteractionDataOption) {
	g, err := s.State.Guild(ic.GuildID)
	if err != nil {
		handler.logger.Info("failed to get guild", zap.Error(err))
		InteractionRespondServerError(handler.logger, s, ic.Interaction)
		return
	}

	player := handler.getGuildPlayer(GuildID(g.ID))

	song, err := player.GetPlayedSong()
	if err != nil {
		handler.logger.Info("failed to played song", zap.Error(err))
		InteractionRespondServerError(handler.logger, s, ic.Interaction)
		return
	}

	if song == nil {
		InteractionRespondMessage(handler.logger, s, ic.Interaction, "🔇 No song is being played right now...")
		return
	}

	InteractionRespondMessage(handler.logger, s, ic.Interaction, fmt.Sprintf("🎶 %s", song.GetHumanName()))
}

func (handler *InteractionHandler) setupGuildPlayer(guildID GuildID) *bot.GuildPlayer {
	dg, err := discordgo.New("Bot " + handler.discordToken)
	if err != nil {
		handler.logger.Error("failed to create Discord session", zap.Error(err))
		return nil
	}

	err = dg.Open()
	if err != nil {
		handler.logger.Error("failed to open Discord session", zap.Error(err))
		return nil
	}

	voiceChat := &DiscordVoiceChatSession{
		discordSession: dg,
		guildID:        string(guildID),
	}

	playlistStore := config.GetPlaylistStore(handler.cfg, string(guildID))

	player := bot.NewGuildPlayer(handler.ctx, voiceChat, string(guildID), playlistStore, sources.GetDCAData).WithLogger(handler.logger.With(zap.String("guildID", string(guildID))))
	return player
}

func (handler *InteractionHandler) getGuildPlayer(guildID GuildID) *bot.GuildPlayer {
	player, ok := handler.guildPlayers[guildID]
	if !ok {
		player = handler.setupGuildPlayer(guildID)
		handler.guildPlayers[guildID] = player
	}

	return player
}
