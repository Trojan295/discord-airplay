package config

type WhisperConfig struct {
	Enabled          bool   `default:"false"`
	ModelPath        string `default:"whisper.cpp/models/ggml-base.en.bin"`
	Threads          int    `default:"0"`
	SamplingStrategy string `default:"beam"`
}

type Config struct {
	DiscordToken  string `required:"true"`
	GuildID       string
	CommandPrefix string `default:"air"`

	Whisper WhisperConfig
}
