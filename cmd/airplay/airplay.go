package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"

	"github.com/Trojan295/discord-airplay/cmd/airplay/commands"
	"github.com/Trojan295/discord-airplay/pkg/asr"
	"github.com/Trojan295/discord-airplay/pkg/bot"
	"github.com/Trojan295/discord-airplay/pkg/config"
	"github.com/Trojan295/discord-airplay/pkg/sources"
	"github.com/bwmarrin/discordgo"
	"github.com/kelseyhightower/envconfig"
)

type GuildID string

var (
	cfg = &config.Config{}

	streamer     *sources.YoutubeFetcher
	guildPlayers map[GuildID]*bot.GuildPlayer
)

func main() {
	if err := envconfig.Process("AIR", cfg); err != nil {
		log.Fatalf("failed to load envconfig: %v", err)
	}

	if cfg.Whisper.Enabled {
		if cfg.Whisper.Threads == 0 {
			cfg.Whisper.Threads = runtime.NumCPU()
		}

		log.Printf("ASR: Whisper processing using %d threads, %s sampling strategy", cfg.Whisper.Threads, cfg.Whisper.SamplingStrategy)
	}

	streamer = sources.NewYoutubeFetcher()
	guildPlayers = make(map[GuildID]*bot.GuildPlayer)

	commandHandler := commands.NewCommandHandler(cfg.CommandPrefix).
		PlayHandler(playSong).
		SkipHandler(skipSong).
		StopHandler(stopPlaying).
		ListHandler(listPlaylist).
		RemoveHandler(removeSong).
		PlayingNowHandler(getPlayingSong).
		JoinHandler(joinChannel).
		LeaveHandler(leaveChannel)

	slashCommands := commandHandler.GetSlashCommands()

	dg, err := discordgo.New("Bot " + cfg.DiscordToken)
	if err != nil {
		fmt.Println("Error creating Discord session: ", err)
		return
	}

	dg.AddHandler(ready)

	dg.AddHandler(guildCreate)

	dg.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		if h, ok := commandHandler.GetHandlers()[i.ApplicationCommandData().Name]; ok {
			h(s, i)
		}
	})

	// We need information about guilds (which includes their channels),
	// messages and voice states.
	dg.Identify.Intents = discordgo.IntentsGuilds | discordgo.IntentsGuildMessages | discordgo.IntentsGuildVoiceStates

	err = dg.Open()
	if err != nil {
		fmt.Println("Error opening Discord session: ", err)
	}
	defer dg.Close()

	registeredCommands := make([]*discordgo.ApplicationCommand, 0, len(slashCommands))
	for _, v := range slashCommands {
		cmd, err := dg.ApplicationCommandCreate(dg.State.User.ID, cfg.GuildID, v)
		if err != nil {
			log.Panicf("Cannot create '%v' command: %v", v.Name, err)
		}
		registeredCommands = append(registeredCommands, cmd)
	}

	defer func() {
		for _, v := range registeredCommands {
			err := dg.ApplicationCommandDelete(dg.State.User.ID, cfg.GuildID, v.ID)
			if err != nil {
				log.Panicf("Cannot delete '%v' command: %v", v.Name, err)
			}
		}
	}()

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

	player := bot.NewGuildPlayer(dg, guildID)

	if cfg.Whisper.Enabled {
		recognizer, err := asr.NewWhisper(asr.WhisperConfig{
			Modelpath:        cfg.Whisper.ModelPath,
			Threads:          cfg.Whisper.Threads,
			SamplingStrategy: asr.WHISPER_BEAM_SAMPLING,
			ASRCallback: func(text string) {
				handleVoiceCommand(player, text)
			},
		})

		if err != nil {
			log.Printf("failed to create recognizer: %v", err)
			return player
		}

		player = player.ASRService(recognizer)
	}

	return player
}

func handleVoiceCommand(player *bot.GuildPlayer, text string) {
	words := strings.Split(
		strings.ToLower(
			strings.ReplaceAll(
				strings.ReplaceAll(text, ".", ""),
				",", "")),
		" ")

	activationIndex := -1

	for idx, word := range words {
		if word == "eric" {
			activationIndex = idx
		}
	}

	log.Printf("detected speech: %v", text)

	if activationIndex < 0 || len(words) <= activationIndex+1 {
		return
	}

	command := words[activationIndex+1:]

	if command[0] == "stop" {
		player.Stop()
	} else if command[0] == "skip" {
		player.SkipSong()
	} else if command[0] == "play" {
		if len(command) == 1 {
			return
		}

		song := sources.ParseYoutubeInput(strings.Join(command, " "), streamer)

		go func(player *bot.GuildPlayer, song bot.Song) {
			metadata, err := song.GetMetadata(context.Background())
			if err != nil {
				log.Printf("failed to get song metadata %s: %v", song.GetHumanName(), err)
			}

			player.SendMessage(fmt.Sprintf("⏯️ Added **%s** - %s to playlist", metadata.Title, metadata.URL))

		}(player, song)

		player.AddSong(nil, nil, song)
	}
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
			song := sources.ParseYoutubeInput(input, streamer)

			if err := s.InteractionRespond(ic.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "Adding song...",
				},
			}); err != nil {
				log.Printf("failed to respond to add song interaction: %v", err)
			}

			go func(song bot.Song) {
				metadata, err := song.GetMetadata(context.Background())

				if err != nil {
					log.Printf("failed to get song metadata %s: %v", song.GetHumanName(), err)
					commands.FollowupMessageCreate(s, ic.Interaction, &discordgo.WebhookParams{
						Content: "😟 Failed to add song",
					})
					return
				}

				if !metadata.Playable {
					commands.FollowupMessageCreate(s, ic.Interaction, &discordgo.WebhookParams{
						Content: "😟 Cannot play the song",
					})
					return
				}

				player.AddSong(&ic.ChannelID, &vs.ChannelID, song)

				commands.FollowupMessageCreate(s, ic.Interaction, &discordgo.WebhookParams{
					Content: fmt.Sprintf("⏯️ Added **%s** - %s to playlist", metadata.Title, metadata.URL),
				})
			}(song)

			return
		}
	}

	commands.InteractionRespondMessage(s, ic.Interaction, "🤷🏽 You are not in a voice channel. Join a voice channel to play a song.")
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
	message := ""

	if len(playlist) == 0 {
		message = "🫙 Playlist is empty"
	} else {
		for idx, song := range playlist {
			message += fmt.Sprintf("%d. %s\n", idx+1, song)
		}

		message = strings.TrimSpace(message)
	}

	commands.InteractionRespondMessage(s, ic.Interaction, message)
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
		commands.InteractionRespondMessage(s, ic.Interaction, fmt.Sprintf("🔇 No song is being played right now..."))
		return
	}

	commands.InteractionRespondMessage(s, ic.Interaction, fmt.Sprintf("🎶 %s", song.GetHumanName()))
}

func joinChannel(s *discordgo.Session, ic *discordgo.InteractionCreate, opt *discordgo.ApplicationCommandInteractionDataOption) {
	if !cfg.Whisper.Enabled {
		commands.InteractionRespondCommandDisabled(s, ic.Interaction)
		return
	}

	g, err := s.State.Guild(ic.GuildID)
	if err != nil {
		log.Printf("failed to get guild %s: %v", ic.GuildID, err)
		commands.InteractionRespondError(s, ic.Interaction)
		return
	}
	player := guildPlayers[GuildID(g.ID)]

	for _, vs := range g.VoiceStates {
		if vs.UserID == ic.Member.User.ID {
			player.JoinVoiceChannel(vs.ChannelID, ic.ChannelID)
			commands.InteractionRespondMessage(s, ic.Interaction, "Joined channel")
			return
		}
	}

	commands.InteractionRespondMessage(s, ic.Interaction, "🤷🏽 You are not in a voice channel. Join a voice channel to play a song.")
}

func leaveChannel(s *discordgo.Session, ic *discordgo.InteractionCreate, opt *discordgo.ApplicationCommandInteractionDataOption) {
	if !cfg.Whisper.Enabled {
		commands.InteractionRespondCommandDisabled(s, ic.Interaction)
		return
	}

	g, err := s.State.Guild(ic.GuildID)
	if err != nil {
		log.Printf("failed to get guild %s: %v", ic.GuildID, err)
		commands.InteractionRespondError(s, ic.Interaction)
		return
	}
	player := guildPlayers[GuildID(g.ID)]
	player.LeaveVoiceChannel()

	commands.InteractionRespondMessage(s, ic.Interaction, "Left channel")
}
