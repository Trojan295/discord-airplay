package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/Trojan295/discord-airplay/pkg/bot"
	"github.com/Trojan295/discord-airplay/pkg/config"
	"github.com/Trojan295/discord-airplay/pkg/discord"
	"github.com/Trojan295/discord-airplay/pkg/sources"
	"github.com/bwmarrin/discordgo"
	"github.com/kelseyhightower/envconfig"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type GuildID string

var (
	logger *zap.Logger

	ctx       context.Context
	cancelCtx context.CancelFunc

	cfg            = &config.Config{}
	youtubeFetcher *sources.YoutubeFetcher

	storage *bot.InMemoryStorage
)

// TODO: store state in etcd
func main() {
	loggerCfg := zap.NewProductionConfig()
	if os.Getenv("AIR_DEBUG") == "1" {
		loggerCfg = zap.NewDevelopmentConfig()
	}

	loggerCfg.EncoderConfig.EncodeTime = zapcore.RFC3339TimeEncoder
	logger, _ = loggerCfg.Build()

	ctx, cancelCtx = context.WithCancel(context.Background())
	defer cancelCtx()

	if err := envconfig.Process("AIR", cfg); err != nil {
		logger.Fatal("failed to load envconfig", zap.Error(err))
	}

	storage = bot.NewInMemoryStorage()
	youtubeFetcher = sources.NewYoutubeFetcher()
	playlistGenerator := sources.NewChatGPTPlaylistGenerator(cfg.OpenAIToken)

	handler := discord.NewInteractionHandler(ctx, cfg.DiscordToken, youtubeFetcher, playlistGenerator, storage).WithLogger(logger.Named("interactionHandler"))
	commandHandler := discord.NewSlashCommandRouter(cfg.CommandPrefix).
		PlayHandler(handler.PlaySong).
		SkipHandler(handler.SkipSong).
		StopHandler(handler.StopPlaying).
		ListHandler(handler.ListPlaylist).
		RemoveHandler(handler.RemoveSong).
		PlayingNowHandler(handler.GetPlayingSong).
		DJHandler(handler.CreatePlaylist).
		AddSongOrPlaylistHandler(handler.AddSongOrPlaylist)

	dg, err := discordgo.New("Bot " + cfg.DiscordToken)
	if err != nil {
		logger.Fatal("failed to create Discord session", zap.Error(err))
		return
	}

	dg.AddHandler(handler.Ready)
	dg.AddHandler(handler.GuildCreate)
	dg.AddHandler(handler.GuildDelete)

	dg.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		switch i.Type {
		case discordgo.InteractionMessageComponent:
			if h, ok := commandHandler.GetComponentHandlers()[i.MessageComponentData().CustomID]; ok {
				h(s, i)
			}

		default:
			if h, ok := commandHandler.GetCommandHandlers()[i.ApplicationCommandData().Name]; ok {
				h(s, i)
			}
		}
	})

	dg.Identify.Intents = discordgo.IntentsGuilds | discordgo.IntentsGuildMessages | discordgo.IntentsGuildVoiceStates

	err = dg.Open()
	if err != nil {
		logger.Fatal("error opening Discord session", zap.Error(err))
	}
	defer dg.Close()

	slashCommands := commandHandler.GetSlashCommands()
	registeredCommands, err := dg.ApplicationCommandBulkOverwrite(dg.State.User.ID, cfg.GuildID, slashCommands)
	if err != nil {
		logger.Fatal("failed to bulk overwriter command", zap.Error(err))
	}

	if cfg.GuildID != "" {
		defer func() {
			for _, cmd := range registeredCommands {
				if err := dg.ApplicationCommandDelete(dg.State.User.ID, cfg.GuildID, cmd.ID); err != nil {
					logger.Error("failed to delete command", zap.String("command", cmd.Name), zap.Error(err))
				}
			}
		}()
	}

	logger.Info("airplay is now running.  Press CTRL-C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc
}
