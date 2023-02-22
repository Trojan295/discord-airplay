package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/Trojan295/discord-airplay/cmd/airplay/commands"
	"github.com/Trojan295/discord-airplay/pkg/bot"
	"github.com/Trojan295/discord-airplay/pkg/config"
	"github.com/Trojan295/discord-airplay/pkg/sources"
	"github.com/bwmarrin/discordgo"
	"github.com/kelseyhightower/envconfig"
)

type GuildID string

var (
	ctx       context.Context
	cancelCtx context.CancelFunc

	cfg          = &config.Config{}
	streamer     *sources.YoutubeFetcher
	guildPlayers map[GuildID]*bot.GuildPlayer

	storage *bot.InMemoryStorage
)

// TODO: refactor main file and standarize responses
// TODO: list - paginate or limit number of songs listed in playlist (Discord limit)
func main() {
	ctx, cancelCtx = context.WithCancel(context.Background())
	defer cancelCtx()

	if err := envconfig.Process("AIR", cfg); err != nil {
		log.Fatalf("failed to load envconfig: %v", err)
	}

	storage = bot.NewInMemoryStorage()

	streamer = sources.NewYoutubeFetcher()
	guildPlayers = make(map[GuildID]*bot.GuildPlayer)

	commandHandler := commands.NewCommandHandler(cfg.CommandPrefix).
		PlayHandler(playSong).
		SkipHandler(skipSong).
		StopHandler(stopPlaying).
		ListHandler(listPlaylist).
		RemoveHandler(removeSong).
		PlayingNowHandler(getPlayingSong).
		AddSongOrPlaylistHandler(addSongOrPlaylist)

	slashCommands := commandHandler.GetSlashCommands()

	dg, err := discordgo.New("Bot " + cfg.DiscordToken)
	if err != nil {
		fmt.Println("Error creating Discord session: ", err)
		return
	}

	dg.AddHandler(ready)

	dg.AddHandler(guildCreate)

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
		fmt.Println("Error opening Discord session: ", err)
	}
	defer dg.Close()

	registeredCommands, err := dg.ApplicationCommandBulkOverwrite(dg.State.User.ID, cfg.GuildID, slashCommands)
	if err != nil {
		log.Fatalf("failed to bulk overwriter command: %v", err)
	}

	if cfg.GuildID != "" {
		defer func() {
			for _, cmd := range registeredCommands {
				if err := dg.ApplicationCommandDelete(dg.State.User.ID, cfg.GuildID, cmd.ID); err != nil {
					log.Printf("failed to delete command %s: %s", cmd.Name, err.Error())
				}
			}
		}()
	}

	fmt.Println("airplay is now running.  Press CTRL-C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc
}

func ready(s *discordgo.Session, event *discordgo.Ready) {
	if err := s.UpdateGameStatus(0, "🕺💃 /air"); err != nil {
		log.Printf("failed to update game status: %v", err)
	}
}

func guildCreate(s *discordgo.Session, event *discordgo.GuildCreate) {
	if event.Guild.Unavailable {
		return
	}

	player := setupGuildPlayer(event.Guild.ID)
	guildPlayers[GuildID(event.Guild.ID)] = player

	log.Printf("connected to guild ID %s", event.Guild.ID)

	go func() {
		if err := player.Run(context.TODO()); err != nil {
			log.Printf("error occured, when player was running: %v", err)
		}
	}()
}

func setupGuildPlayer(guildID string) *bot.GuildPlayer {
	dg, err := discordgo.New("Bot " + cfg.DiscordToken)
	if err != nil {
		fmt.Println("Error creating Discord session: ", err)
		return nil
	}

	err = dg.Open()
	if err != nil {
		fmt.Println("Error opening Discord session: ", err)
	}

	player := bot.NewGuildPlayer(ctx, dg, guildID)

	return player
}

func playSong(s *discordgo.Session, ic *discordgo.InteractionCreate, opt *discordgo.ApplicationCommandInteractionDataOption) {
	g, err := s.State.Guild(ic.GuildID)
	if err != nil {
		log.Printf("failed to get guild %s: %v", ic.GuildID, err)
		commands.InteractionRespondError(s, ic.Interaction)
		return
	}
	player := guildPlayers[GuildID(g.ID)]

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
					Content: "⏳ Adding song...",
				},
			}); err != nil {
				log.Printf("failed to respond to add song interaction: %v", err)
			}

			go func() {
				songs, err := streamer.LookupSongs(ctx, input)
				if err != nil {
					log.Printf("failed to lookup song metadata: %v", err)
					commands.FollowupMessageCreate(s, ic.Interaction, &discordgo.WebhookParams{
						Content: "😨 Failed to add song",
					})
				}

				if err != nil {
					log.Printf("failed to get song metadata for '%s': %v", input, err)
					commands.FollowupMessageCreate(s, ic.Interaction, &discordgo.WebhookParams{
						Content: "😨 Failed to add song",
					})
					return
				}

				if len(songs) == 0 {
					commands.FollowupMessageCreate(s, ic.Interaction, &discordgo.WebhookParams{
						Content: "😨 Could not find any playable songs",
					})
					return
				}

				if len(songs) == 1 {
					song := songs[0]
					metadata := song.GetMetadata()

					player.AddSong(&ic.ChannelID, &vs.ChannelID, song)

					commands.FollowupMessageCreate(s, ic.Interaction, &discordgo.WebhookParams{
						Content: fmt.Sprintf("➕ Added **%s** - %s to playlist", metadata.Title, metadata.URL),
					})
				} else {
					storage.PutSongs(ic.ChannelID, songs)

					commands.FollowupMessageCreate(s, ic.Interaction, &discordgo.WebhookParams{
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

	commands.InteractionRespondMessage(s, ic.Interaction, "🤷🏽 You are not in a voice channel. Join a voice channel to play a song.")
}

func addSongOrPlaylist(s *discordgo.Session, ic *discordgo.InteractionCreate) {
	values := ic.MessageComponentData().Values
	if len(values) == 0 {
		commands.InteractionRespondMessage(s, ic.Interaction, "😨 Something went wrong...")
		return
	}

	g, err := s.State.Guild(ic.GuildID)
	if err != nil {
		log.Printf("failed to get guild %s: %v", ic.GuildID, err)
		commands.InteractionRespondError(s, ic.Interaction)
		return
	}

	value := values[0]
	songs := storage.GetSongs(ic.ChannelID)
	player := guildPlayers[GuildID(g.ID)]

	var voiceChannelID *string = nil

	for _, vs := range g.VoiceStates {
		if vs.UserID == ic.Member.User.ID {
			voiceChannelID = &vs.ChannelID
			break
		}
	}

	if voiceChannelID == nil {
		commands.InteractionRespondMessage(s, ic.Interaction, "🤷🏽 You are not in a voice channel. Join a voice channel to play a song.")
		return
	}

	switch value {
	case "playlist":
		for _, song := range songs {
			player.AddSong(&ic.Message.ChannelID, voiceChannelID, song)
		}
		commands.InteractionRespondMessage(s, ic.Interaction, fmt.Sprintf("➕ Added %d songs to playlist", len(songs)))
	default:
		song := songs[0]
		metadata := song.GetMetadata()
		player.AddSong(&ic.Message.ChannelID, voiceChannelID, song)
		commands.InteractionRespondMessage(s, ic.Interaction, fmt.Sprintf("➕ Added **%s** - %s to playlist", metadata.Title, metadata.URL))
	}

	storage.DeleteSongsKey(ic.ChannelID)
}

func skipSong(s *discordgo.Session, ic *discordgo.InteractionCreate, acido *discordgo.ApplicationCommandInteractionDataOption) {
	g, err := s.State.Guild(ic.GuildID)
	if err != nil {
		log.Printf("failed to get guild %s: %v", ic.GuildID, err)
		commands.InteractionRespondError(s, ic.Interaction)
		return
	}
	player := guildPlayers[GuildID(g.ID)]
	player.SkipSong()

	commands.InteractionRespondMessage(s, ic.Interaction, "⏭️ Skipped song")
}

func stopPlaying(s *discordgo.Session, ic *discordgo.InteractionCreate, acido *discordgo.ApplicationCommandInteractionDataOption) {
	g, err := s.State.Guild(ic.GuildID)
	if err != nil {
		log.Printf("failed to get guild %s: %v", ic.GuildID, err)
		commands.InteractionRespondError(s, ic.Interaction)
		return
	}
	player := guildPlayers[GuildID(g.ID)]
	player.Stop()

	commands.InteractionRespondMessage(s, ic.Interaction, "⏹️ Stopped playing!")
}

func listPlaylist(s *discordgo.Session, ic *discordgo.InteractionCreate, acido *discordgo.ApplicationCommandInteractionDataOption) {
	g, err := s.State.Guild(ic.GuildID)
	if err != nil {
		log.Printf("failed to get guild %s: %v", ic.GuildID, err)
		commands.InteractionRespondError(s, ic.Interaction)
		return
	}
	player := guildPlayers[GuildID(g.ID)]

	playlist := player.GetPlaylist()

	if len(playlist) == 0 {
		commands.InteractionRespondMessage(s, ic.Interaction, "🫙 Playlist is empty")
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

		commands.InteractionRespond(s, ic.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Embeds: []*discordgo.MessageEmbed{
					{Title: "Playlist:", Description: message},
				},
			},
		})
	}
}

func removeSong(s *discordgo.Session, ic *discordgo.InteractionCreate, opt *discordgo.ApplicationCommandInteractionDataOption) {
	g, err := s.State.Guild(ic.GuildID)
	if err != nil {
		log.Printf("failed to get guild %s: %v", ic.GuildID, err)
		commands.InteractionRespondError(s, ic.Interaction)
		return
	}
	player := guildPlayers[GuildID(g.ID)]

	optionMap := make(map[string]*discordgo.ApplicationCommandInteractionDataOption, len(opt.Options))
	for _, opt := range opt.Options {
		optionMap[opt.Name] = opt
	}

	position := optionMap["position"].IntValue()

	song, err := player.RemoveSong(int(position))
	if err != nil {
		commands.InteractionRespondError(s, ic.Interaction)
	}

	commands.InteractionRespondMessage(s, ic.Interaction, fmt.Sprintf("🗑️ Removed song **%v** from playlist", song.GetHumanName()))
}

func getPlayingSong(s *discordgo.Session, ic *discordgo.InteractionCreate, opt *discordgo.ApplicationCommandInteractionDataOption) {
	g, err := s.State.Guild(ic.GuildID)
	if err != nil {
		log.Printf("failed to get guild %s: %v", ic.GuildID, err)
		commands.InteractionRespondError(s, ic.Interaction)
		return
	}
	player := guildPlayers[GuildID(g.ID)]

	song := player.GetPlayedSong()
	if song == nil {
		commands.InteractionRespondMessage(s, ic.Interaction, "🔇 No song is being played right now...")
		return
	}

	commands.InteractionRespondMessage(s, ic.Interaction, fmt.Sprintf("🎶 %s", song.GetHumanName()))
}
