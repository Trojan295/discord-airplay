//go:build vosk
// +build vosk

package asr

import (
	"encoding/json"
	"fmt"

	"github.com/Trojan295/discord-airplay/pkg/codec"
	vosk "github.com/alphacep/vosk-api/go"
)

type VoskRecognition struct {
	voskRec    *vosk.VoskRecognizer
	sampleRate float32

	asrCallback func(command string)
}

type VoskResult struct {
	Text string `json:"text"`
}

func NewVoskRecognition(model string, sampleRate float32, askCallback func(string)) (*VoskRecognition, error) {
	voskModel, err := vosk.NewModel(model)
	if err != nil {
		return nil, fmt.Errorf("while creating Vosk model: %v", err)
	}

	rec, err := vosk.NewRecognizer(voskModel, float64(sampleRate))
	if err != nil {
		return nil, fmt.Errorf("while creating Vosk recognizer: %v", err)
	}

	return &VoskRecognition{
		voskRec:     rec,
		sampleRate:  sampleRate,
		asrCallback: askCallback,
	}, nil
}

func (r *VoskRecognition) FeedOpusData(opusData []byte) error {
	pcm, err := codec.ConvertOpusToPCM(float64(r.sampleRate), 1, opusData)
	if err != nil {
		return fmt.Errorf("while converting data to PCM: %v", err)
	}

	if r.voskRec.AcceptWaveform(pcm) != 0 {
		result := &VoskResult{}
		if err := json.Unmarshal([]byte(r.voskRec.Result()), result); err != nil {
			return fmt.Errorf("while unmarshaling Vosk result: %v", err)
		}

		r.asrCallback(result.Text)
	}

	return nil
}
