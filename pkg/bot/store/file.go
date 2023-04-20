package store

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"github.com/Trojan295/discord-airplay/pkg/bot"
)

type FilePlaylistStorage struct {
	mutex    sync.RWMutex
	filepath string
}

func NewFilePlaylistStorage(filepath string) (*FilePlaylistStorage, error) {
	if _, err := os.Stat(filepath); os.IsNotExist(err) {
		if err := os.WriteFile(filepath, []byte("[]"), 0644); err != nil {
			return nil, fmt.Errorf("failed to create file: %w", err)
		}
	}

	return &FilePlaylistStorage{
		mutex:    sync.RWMutex{},
		filepath: filepath,
	}, nil
}

func (s *FilePlaylistStorage) AppendSong(song bot.Song) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	songs, err := s.readSongs()
	if err != nil {
		return fmt.Errorf("failed to read songs: %w", err)
	}

	songs = append(songs, song)
	if err := s.writeSongs(songs); err != nil {
		return fmt.Errorf("failed to write songs: %w", err)
	}

	return nil
}

func (s *FilePlaylistStorage) RemoveSong(position int) (bot.Song, error) {
	index := position - 1

	s.mutex.Lock()
	defer s.mutex.Unlock()

	songs, err := s.readSongs()
	if err != nil {
		return nil, fmt.Errorf("failed to read songs: %w", err)
	}

	if index >= len(songs) || index < 0 {
		return nil, bot.ErrRemoveInvalidPosition
	}

	song := songs[index]

	copy(songs[index:], songs[index+1:])
	songs[len(songs)-1] = nil
	songs = songs[:len(songs)-1]

	if err := s.writeSongs(songs); err != nil {
		return nil, fmt.Errorf("failed to write songs: %w", err)
	}

	return song, nil
}

func (s *FilePlaylistStorage) ClearPlaylist() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	songs := make([]bot.Song, 0)

	if err := s.writeSongs(songs); err != nil {
		return fmt.Errorf("failed to write songs: %w", err)
	}

	return nil
}

func (s *FilePlaylistStorage) GetSongs() ([]bot.Song, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	songs, err := s.readSongs()
	if err != nil {
		return nil, fmt.Errorf("failed to read songs: %w", err)
	}

	return songs, nil
}

func (s *FilePlaylistStorage) PopFirstSong() (bot.Song, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	songs, err := s.readSongs()
	if err != nil {
		return nil, fmt.Errorf("failed to read songs: %w", err)
	}

	if len(songs) == 0 {
		return nil, bot.ErrNoSongs
	}

	song := songs[0]
	songs = songs[1:]

	if err := s.writeSongs(songs); err != nil {
		return nil, fmt.Errorf("failed to write songs: %w", err)
	}

	return song, nil
}

func (s *FilePlaylistStorage) readSongs() ([]bot.Song, error) {
	data, err := os.ReadFile(s.filepath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	var songs []*bot.Song
	if err := json.Unmarshal(data, &songs); err != nil {
		return nil, fmt.Errorf("failed to unmarshal songs: %w", err)
	}

	return nil, nil
}

func (s *FilePlaylistStorage) writeSongs(songs []bot.Song) error {
	data, err := json.Marshal(songs)
	if err != nil {
		return fmt.Errorf("failed to marshal songs: %w", err)
	}

	if err := os.WriteFile(s.filepath, data, 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}
	return nil
}
