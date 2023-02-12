package commands

import (
	"github.com/bwmarrin/discordgo"
)

type CommandHandler struct {
	commandPrefix string

	playHandler       func(*discordgo.Session, *discordgo.InteractionCreate, *discordgo.ApplicationCommandInteractionDataOption)
	stopHandler       func(*discordgo.Session, *discordgo.InteractionCreate, *discordgo.ApplicationCommandInteractionDataOption)
	listHandler       func(*discordgo.Session, *discordgo.InteractionCreate, *discordgo.ApplicationCommandInteractionDataOption)
	skipHandler       func(*discordgo.Session, *discordgo.InteractionCreate, *discordgo.ApplicationCommandInteractionDataOption)
	removeHandler     func(*discordgo.Session, *discordgo.InteractionCreate, *discordgo.ApplicationCommandInteractionDataOption)
	joinHandler       func(*discordgo.Session, *discordgo.InteractionCreate, *discordgo.ApplicationCommandInteractionDataOption)
	leaveHandler      func(*discordgo.Session, *discordgo.InteractionCreate, *discordgo.ApplicationCommandInteractionDataOption)
	playingNowHandler func(*discordgo.Session, *discordgo.InteractionCreate, *discordgo.ApplicationCommandInteractionDataOption)
}

func NewCommandHandler(commandPrefix string) *CommandHandler {
	return &CommandHandler{
		commandPrefix: commandPrefix,
	}
}

func (ch *CommandHandler) PlayHandler(h func(*discordgo.Session, *discordgo.InteractionCreate, *discordgo.ApplicationCommandInteractionDataOption)) *CommandHandler {
	ch.playHandler = h
	return ch
}

func (ch *CommandHandler) StopHandler(h func(*discordgo.Session, *discordgo.InteractionCreate, *discordgo.ApplicationCommandInteractionDataOption)) *CommandHandler {
	ch.stopHandler = h
	return ch
}

func (ch *CommandHandler) SkipHandler(h func(*discordgo.Session, *discordgo.InteractionCreate, *discordgo.ApplicationCommandInteractionDataOption)) *CommandHandler {
	ch.skipHandler = h
	return ch
}

func (ch *CommandHandler) ListHandler(h func(*discordgo.Session, *discordgo.InteractionCreate, *discordgo.ApplicationCommandInteractionDataOption)) *CommandHandler {
	ch.listHandler = h
	return ch
}

func (ch *CommandHandler) RemoveHandler(h func(*discordgo.Session, *discordgo.InteractionCreate, *discordgo.ApplicationCommandInteractionDataOption)) *CommandHandler {
	ch.removeHandler = h
	return ch
}

func (ch *CommandHandler) JoinHandler(h func(*discordgo.Session, *discordgo.InteractionCreate, *discordgo.ApplicationCommandInteractionDataOption)) *CommandHandler {
	ch.joinHandler = h
	return ch
}

func (ch *CommandHandler) LeaveHandler(h func(*discordgo.Session, *discordgo.InteractionCreate, *discordgo.ApplicationCommandInteractionDataOption)) *CommandHandler {
	ch.leaveHandler = h
	return ch
}

func (ch *CommandHandler) PlayingNowHandler(h func(*discordgo.Session, *discordgo.InteractionCreate, *discordgo.ApplicationCommandInteractionDataOption)) *CommandHandler {
	ch.playingNowHandler = h
	return ch
}

func (ch *CommandHandler) GetHandlers() map[string]func(*discordgo.Session, *discordgo.InteractionCreate) {
	return map[string]func(*discordgo.Session, *discordgo.InteractionCreate){
		ch.commandPrefix: func(s *discordgo.Session, ic *discordgo.InteractionCreate) {
			options := ic.ApplicationCommandData().Options
			option := options[0]

			switch option.Name {
			case "play":
				ch.playHandler(s, ic, option)
			case "stop":
				ch.stopHandler(s, ic, option)
			case "list":
				ch.listHandler(s, ic, option)
			case "skip":
				ch.skipHandler(s, ic, option)
			case "remove":
				ch.removeHandler(s, ic, option)
			case "playing":
				ch.playingNowHandler(s, ic, option)
			case "join":
				ch.joinHandler(s, ic, option)
			case "leave":
				ch.leaveHandler(s, ic, option)
			}
		},
	}
}

func (ch *CommandHandler) GetSlashCommands() []*discordgo.ApplicationCommand {
	return []*discordgo.ApplicationCommand{
		{
			Name:        ch.commandPrefix,
			Description: "Air command",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "play",
					Description: "Add a song to the playlist",
					Options: []*discordgo.ApplicationCommandOption{
						{
							Type:        discordgo.ApplicationCommandOptionString,
							Name:        "input",
							Description: "URL or name of the track",
							Required:    true,
						},
					},
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "remove",
					Description: "Remove song from playlist",
					Options: []*discordgo.ApplicationCommandOption{
						{
							Type:        discordgo.ApplicationCommandOptionInteger,
							Name:        "position",
							Description: "Position of the song in the playlist",
							Required:    true,
						},
					},
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "skip",
					Description: "Skip the current song",
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "stop",
					Description: "Stop playing and clear playlist",
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "list",
					Description: "List the playlist",
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "playing",
					Description: "Get currently playing song",
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "join",
					Description: "Make airplay join the voice channel where you are",
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "leave",
					Description: "Make airplay to leave the voice channel",
				},
			},
		},
	}
}
