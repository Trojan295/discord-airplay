package store

import (
	"sync"

	"github.com/Trojan295/discord-airplay/pkg/bot"
)

type InmemoryPlaylistStorage struct {
	mutex sync.RWMutex
	songs []bot.Song
}

func NewInmemoryPlaylistStorage() *InmemoryPlaylistStorage {
	return &InmemoryPlaylistStorage{
		mutex: sync.RWMutex{},
		songs: make([]bot.Song, 0),
	}
}

func (s *InmemoryPlaylistStorage) AppendSong(song bot.Song) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.songs = append(s.songs, song)
	return nil
}

func (s *InmemoryPlaylistStorage) RemoveSong(position int) (bot.Song, error) {
	index := position - 1

	s.mutex.Lock()
	defer s.mutex.Unlock()

	if index >= len(s.songs) || index < 0 {
		return nil, bot.ErrRemoveInvalidPosition
	}

	song := s.songs[index]

	copy(s.songs[index:], s.songs[index+1:])
	s.songs[len(s.songs)-1] = nil
	s.songs = s.songs[:len(s.songs)-1]
	return song, nil
}

func (s *InmemoryPlaylistStorage) ClearPlaylist() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.songs = make([]bot.Song, 0)
	return nil
}

func (s *InmemoryPlaylistStorage) GetSongs() ([]bot.Song, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	songs := make([]bot.Song, len(s.songs))
	copy(songs, s.songs)

	return s.songs, nil
}

func (s *InmemoryPlaylistStorage) PopFirstSong() (bot.Song, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if len(s.songs) == 0 {
		return nil, bot.ErrNoSongs
	}

	song := s.songs[0]
	s.songs = s.songs[1:]

	return song, nil
}
