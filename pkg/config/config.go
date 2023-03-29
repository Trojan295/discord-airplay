package config

type Config struct {
	DiscordToken  string `required:"true"`
	OpenAIToken   string
	GuildID       string
	CommandPrefix string `default:"air"`
}
