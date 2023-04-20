package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Trojan295/discord-airplay/pkg/bot"
	"github.com/Trojan295/discord-airplay/pkg/bot/store"
)

type Config struct {
	DiscordToken  string `required:"true"`
	OpenAIToken   string
	GuildID       string
	CommandPrefix string `default:"air"`

	Store StoreConfig
}

type StoreConfig struct {
	Type string `default:"memory"`
	File FileStoreConfig
}

type FileStoreConfig struct {
	Dir string `default:"./playlist"`
}

func GetPlaylistStore(cfg *Config, guildID string) bot.PlaylistStore {
	fmt.Println("GetPlaylistStore: ", cfg.Store.Type)

	switch cfg.Store.Type {
	case "memory":
		return store.NewInmemoryPlaylistStorage()
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
