package discord

import (
	"context"
	"fmt"
	"strings"

	"github.com/Trojan295/discord-airplay/pkg/bot"
	"github.com/bwmarrin/discordgo"
	"go.uber.org/zap"
)

type GuildID string

type SongLookuper interface {
	LookupSongs(ctx context.Context, input string) ([]bot.Song, error)
}

type InteractionHandler struct {
	ctx          context.Context
	discordToken string

	guildPlayers map[GuildID]*bot.GuildPlayer

	songLookuper SongLookuper
	storage      *bot.InMemoryStorage

	logger *zap.Logger
}

func NewInteractionHandler(ctx context.Context, discordToken string, songLookuper SongLookuper, storage *bot.InMemoryStorage) *InteractionHandler {
	handler := &InteractionHandler{
		ctx:          ctx,
		discordToken: discordToken,
		guildPlayers: make(map[GuildID]*bot.GuildPlayer),
		songLookuper: songLookuper,
		storage:      storage,
		logger:       zap.NewNop(),
	}

	return handler
}

func (handler *InteractionHandler) WithLogger(l *zap.Logger) *InteractionHandler {
	handler.logger = l
	return handler
}

func (handler *InteractionHandler) Ready(s *discordgo.Session, event *discordgo.Ready) {
	if err := s.UpdateGameStatus(0, "πΊπ /air"); err != nil {
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
			if err := s.InteractionRespond(ic.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "β³ Adding song...",
				},
			}); err != nil {
				logger.Info("failed to respond to add song interaction", zap.Error(err))
			}

			go func() {
				songs, err := handler.songLookuper.LookupSongs(handler.ctx, input)
				if err != nil {
					logger.Info("failed to lookup song metadata", zap.Error(err))
					FollowupMessageCreate(handler.logger, s, ic.Interaction, &discordgo.WebhookParams{
						Content: "π¨ Failed to add song",
					})
				}

				if err != nil {
					logger.Info("failed to get song metadata", zap.String("songInput", input), zap.Error(err))
					FollowupMessageCreate(handler.logger, s, ic.Interaction, &discordgo.WebhookParams{
						Content: "π¨ Failed to add song",
					})
					return
				}

				if len(songs) == 0 {
					FollowupMessageCreate(handler.logger, s, ic.Interaction, &discordgo.WebhookParams{
						Content: "π¨ Could not find any playable songs",
					})
					return
				}

				if len(songs) == 1 {
					song := songs[0]
					metadata := song.GetMetadata()

					player.AddSong(&ic.ChannelID, &vs.ChannelID, song)

					FollowupMessageCreate(handler.logger, s, ic.Interaction, &discordgo.WebhookParams{
						Content: fmt.Sprintf("β Added **%s** - %s to playlist", metadata.Title, metadata.URL),
					})
				} else {
					handler.storage.PutSongs(ic.ChannelID, songs)

					FollowupMessageCreate(handler.logger, s, ic.Interaction, &discordgo.WebhookParams{
						Content: fmt.Sprintf("π The song is part of a playlist, which contains %d songs. What should I do?", len(songs)),
						Components: []discordgo.MessageComponent{
							discordgo.ActionsRow{
								Components: []discordgo.MessageComponent{
									discordgo.SelectMenu{
										CustomID: "add_song_playlist",
										Options: []discordgo.SelectMenuOption{
											{Label: "Add song", Value: "song", Emoji: discordgo.ComponentEmoji{Name: "π΅"}},
											{Label: "Add whole playlist", Value: "playlist", Emoji: discordgo.ComponentEmoji{Name: "πΆ"}},
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

	InteractionRespondMessage(handler.logger, s, ic.Interaction, "π€·π½ You are not in a voice channel. Join a voice channel to play a song.")
}

func (handler *InteractionHandler) AddSongOrPlaylist(s *discordgo.Session, ic *discordgo.InteractionCreate) {
	values := ic.MessageComponentData().Values
	if len(values) == 0 {
		InteractionRespondMessage(handler.logger, s, ic.Interaction, "π¨ Something went wrong...")
		return
	}

	g, err := s.State.Guild(ic.GuildID)
	if err != nil {
		handler.logger.Info("failed to get guild", zap.Error(err))
		InteractionRespondServerError(handler.logger, s, ic.Interaction)
		return
	}

	value := values[0]
	songs := handler.storage.GetSongs(ic.ChannelID)
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
		InteractionRespondMessage(handler.logger, s, ic.Interaction, "π€·π½ You are not in a voice channel. Join a voice channel to play a song.")
		return
	}

	switch value {
	case "playlist":
		for _, song := range songs {
			player.AddSong(&ic.Message.ChannelID, voiceChannelID, song)
		}
		InteractionRespondMessage(handler.logger, s, ic.Interaction, fmt.Sprintf("β Added %d songs to playlist", len(songs)))
	default:
		song := songs[0]
		metadata := song.GetMetadata()
		player.AddSong(&ic.Message.ChannelID, voiceChannelID, song)
		InteractionRespondMessage(handler.logger, s, ic.Interaction, fmt.Sprintf("β Added **%s** - %s to playlist", metadata.Title, metadata.URL))
	}

	handler.storage.DeleteSongsKey(ic.ChannelID)
}

func (handler *InteractionHandler) StopPlaying(s *discordgo.Session, ic *discordgo.InteractionCreate, acido *discordgo.ApplicationCommandInteractionDataOption) {
	g, err := s.State.Guild(ic.GuildID)
	if err != nil {
		handler.logger.Info("failed to get guild", zap.Error(err))
		InteractionRespondServerError(handler.logger, s, ic.Interaction)
		return
	}

	player := handler.getGuildPlayer(GuildID(g.ID))
	player.Stop()

	InteractionRespondMessage(handler.logger, s, ic.Interaction, "βΉοΈ Stopped playing!")
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

	InteractionRespondMessage(handler.logger, s, ic.Interaction, "β­οΈ Skipped song")
}

func (handler *InteractionHandler) ListPlaylist(s *discordgo.Session, ic *discordgo.InteractionCreate, acido *discordgo.ApplicationCommandInteractionDataOption) {
	g, err := s.State.Guild(ic.GuildID)
	if err != nil {
		handler.logger.Info("failed to get guild", zap.Error(err))
		InteractionRespondServerError(handler.logger, s, ic.Interaction)
		return
	}

	player := handler.getGuildPlayer(GuildID(g.ID))
	playlist := player.GetPlaylist()

	if len(playlist) == 0 {
		InteractionRespondMessage(handler.logger, s, ic.Interaction, "π« Playlist is empty")
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
		InteractionRespondServerError(handler.logger, s, ic.Interaction)
	}

	InteractionRespondMessage(handler.logger, s, ic.Interaction, fmt.Sprintf("ποΈ Removed song **%v** from playlist", song.GetHumanName()))
}

func (handler *InteractionHandler) GetPlayingSong(s *discordgo.Session, ic *discordgo.InteractionCreate, opt *discordgo.ApplicationCommandInteractionDataOption) {
	g, err := s.State.Guild(ic.GuildID)
	if err != nil {
		handler.logger.Info("failed to get guild", zap.Error(err))
		InteractionRespondServerError(handler.logger, s, ic.Interaction)
		return
	}

	player := handler.getGuildPlayer(GuildID(g.ID))

	song := player.GetPlayedSong()
	if song == nil {
		InteractionRespondMessage(handler.logger, s, ic.Interaction, "π No song is being played right now...")
		return
	}

	InteractionRespondMessage(handler.logger, s, ic.Interaction, fmt.Sprintf("πΆ %s", song.GetHumanName()))
}

func (handler *InteractionHandler) setupGuildPlayer(guildID GuildID) *bot.GuildPlayer {
	dg, err := discordgo.New("Bot " + handler.discordToken)
	if err != nil {
		fmt.Println("Error creating Discord session: ", err)
		return nil
	}

	err = dg.Open()
	if err != nil {
		fmt.Println("Error opening Discord session: ", err)
	}

	voiceChat := &DiscordVoiceChatSession{
		discordSession: dg,
		guildID:        string(guildID),
	}

	player := bot.NewGuildPlayer(handler.ctx, voiceChat, string(guildID))
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
