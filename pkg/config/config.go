package config

import (
	"os"
	"path/filepath"

	"github.com/Trojan295/discord-airplay/pkg/bot"
	"github.com/Trojan295/discord-airplay/pkg/bot/store"
)

type Config struct {
	DiscordToken  string `required:"true"`
	OpenAIToken   string
	CommandPrefix string `default:"air"`

	PerGuildCommands bool `default:"false"`

	Store StoreConfig

	YtDlp YtDlpConfig
}

type StoreConfig struct {
	Type string `default:"memory"`
	File FileStoreConfig
}

type YtDlpConfig struct {
	Proxy string `default:""`
}

type FileStoreConfig struct {
	Dir string `default:"./playlist"`
}

func GetPlaylistStore(cfg *Config, guildID string) bot.GuildPlayerState {
	switch cfg.Store.Type {
	case "memory":
		return store.NewInmemoryGuildPlayerState()
	case "file":
		if err := os.MkdirAll(cfg.Store.File.Dir, 0755); err != nil {
			panic(err)
		}

		path := filepath.Join(cfg.Store.File.Dir, guildID+".json")
		s, err := store.NewFilePlaylistStorage(path)
		if err != nil {
			panic(err)
		}

		return s

	default:
		panic("invalid store type")
	}
}
