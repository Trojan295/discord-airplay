package bot

type InMemoryStorage struct {
	songsToAdd map[string][]Song
}

func NewInMemoryStorage() *InMemoryStorage {
	return &InMemoryStorage{
		songsToAdd: make(map[string][]Song),
	}
}

func (s *InMemoryStorage) PutSongs(key string, songs []Song) {
	s.songsToAdd[key] = songs
}

func (s *InMemoryStorage) DeleteSongsKey(key string) {
	delete(s.songsToAdd, key)
}

func (s *InMemoryStorage) GetSongs(key string) []Song {
	return s.songsToAdd[key]
}
