package main

import (
	"encoding/json"
	"fmt"
)

type song interface {
	GetTitle() string
}

type isong struct {
	Title string `json:"title"`
}

func (s *isong) GetTitle() string {
	return s.Title
}

func main() {
	songs := []song{
		&isong{Title: "song1"},
		&isong{Title: "song2"},
	}

	data, err := json.Marshal(songs)
	fmt.Println(string(data), err)
}
