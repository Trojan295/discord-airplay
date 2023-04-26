package store

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"github.com/Trojan295/discord-airplay/pkg/bot"
)

type fileState struct {
	Songs        []*bot.Song     `json:"songs"`
	CurrentSong  *bot.PlayedSong `json:"current_song"`
	VoiceChannel string          `json:"voice_channel"`
	TextChannel  string          `json:"text_channel"`
}

type FilePlaylistStorage struct {
	mutex    sync.RWMutex
	filepath string
}

func NewFilePlaylistStorage(filepath string) (*FilePlaylistStorage, error) {
	if _, err := os.Stat(filepath); os.IsNotExist(err) {
		if err := os.WriteFile(filepath, []byte("{}"), 0644); err != nil {
			return nil, fmt.Errorf("failed to create file: %w", err)
		}
	}

	return &FilePlaylistStorage{
		mutex:    sync.RWMutex{},
		filepath: filepath,
	}, nil
}

func (s *FilePlaylistStorage) GetCurrentSong() (*bot.PlayedSong, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	state, err := s.readState()
	if err != nil {
		return nil, fmt.Errorf("failed to read state: %w", err)
	}

	return state.CurrentSong, nil
}

func (s *FilePlaylistStorage) SetCurrentSong(song *bot.PlayedSong) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	state, err := s.readState()
	if err != nil {
		return fmt.Errorf("failed to read state: %w", err)
	}

	state.CurrentSong = song

	if err := s.writeState(state); err != nil {
		return fmt.Errorf("failed to write state: %w", err)
	}

	return nil
}

func (s *FilePlaylistStorage) GetVoiceChannel() (string, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	state, err := s.readState()
	if err != nil {
		return "", fmt.Errorf("failed to read state: %w", err)
	}

	return state.VoiceChannel, nil
}

func (s *FilePlaylistStorage) SetVoiceChannel(channelID string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	state, err := s.readState()
	if err != nil {
		return fmt.Errorf("failed to read state: %w", err)
	}

	state.VoiceChannel = channelID

	if err := s.writeState(state); err != nil {
		return fmt.Errorf("failed to write state: %w", err)
	}

	return nil
}

func (s *FilePlaylistStorage) GetTextChannel() (string, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	state, err := s.readState()
	if err != nil {
		return "", fmt.Errorf("failed to read state: %w", err)
	}

	return state.TextChannel, nil
}

func (s *FilePlaylistStorage) SetTextChannel(channelID string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	state, err := s.readState()
	if err != nil {
		return fmt.Errorf("failed to read state: %w", err)
	}

	state.TextChannel = channelID

	if err := s.writeState(state); err != nil {
		return fmt.Errorf("failed to write state: %w", err)
	}

	return nil
}

func (s *FilePlaylistStorage) PrependSong(song *bot.Song) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	state, err := s.readState()
	if err != nil {
		return fmt.Errorf("failed to read state: %w", err)
	}

	state.Songs = append([]*bot.Song{song}, state.Songs...)

	if err := s.writeState(state); err != nil {
		return fmt.Errorf("failed to write state: %w", err)
	}

	return nil
}

func (s *FilePlaylistStorage) AppendSong(song *bot.Song) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	state, err := s.readState()
	if err != nil {
		return fmt.Errorf("failed to read state: %w", err)
	}

	state.Songs = append(state.Songs, song)

	if err := s.writeState(state); err != nil {
		return fmt.Errorf("failed to write state: %w", err)
	}

	return nil
}

func (s *FilePlaylistStorage) RemoveSong(position int) (*bot.Song, error) {
	index := position - 1

	s.mutex.Lock()
	defer s.mutex.Unlock()

	state, err := s.readState()
	if err != nil {
		return nil, fmt.Errorf("failed to read state: %w", err)
	}

	if index >= len(state.Songs) || index < 0 {
		return nil, bot.ErrRemoveInvalidPosition
	}

	song := state.Songs[index]

	copy(state.Songs[index:], state.Songs[index+1:])
	state.Songs[len(state.Songs)-1] = nil
	state.Songs = state.Songs[:len(state.Songs)-1]

	if err := s.writeState(state); err != nil {
		return nil, fmt.Errorf("failed to write state: %w", err)
	}

	return song, nil
}

func (s *FilePlaylistStorage) ClearPlaylist() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	state, err := s.readState()
	if err != nil {
		return fmt.Errorf("failed to read state: %w", err)
	}

	state.Songs = make([]*bot.Song, 0)

	if err := s.writeState(state); err != nil {
		return fmt.Errorf("failed to write state: %w", err)
	}

	return nil
}

func (s *FilePlaylistStorage) GetSongs() ([]*bot.Song, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	state, err := s.readState()
	if err != nil {
		return nil, fmt.Errorf("failed to read state: %w", err)
	}

	return state.Songs, nil
}

func (s *FilePlaylistStorage) PopFirstSong() (*bot.Song, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	state, err := s.readState()
	if err != nil {
		return nil, fmt.Errorf("failed to read songs: %w", err)
	}

	if len(state.Songs) == 0 {
		return nil, bot.ErrNoSongs
	}

	song := state.Songs[0]
	state.Songs = state.Songs[1:]

	if err := s.writeState(state); err != nil {
		return nil, fmt.Errorf("failed to write songs: %w", err)
	}

	return song, nil
}

func (s *FilePlaylistStorage) readState() (*fileState, error) {
	data, err := os.ReadFile(s.filepath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	var state fileState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to unmarshal songs: %w", err)
	}

	return &state, nil
}

func (s *FilePlaylistStorage) writeState(state *fileState) error {
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	if err := os.WriteFile(s.filepath, data, 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}
	return nil
}
