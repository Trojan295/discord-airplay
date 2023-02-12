//go:build whisper
// +build whisper

package asr

import (
	"fmt"
	"time"

	"github.com/Trojan295/discord-airplay/pkg/codec"
	whisper "github.com/ggerganov/whisper.cpp/bindings/go"
)

type WhisperSampling string

const (
	WHISPER_BEAM_SAMPLING   WhisperSampling = "beam"
	WHISPER_GREEDY_SAMPLING WhisperSampling = "greedy"
)

var (
	whisperSamplingStrategies map[WhisperSampling]whisper.SamplingStrategy = map[WhisperSampling]whisper.SamplingStrategy{
		WHISPER_BEAM_SAMPLING:   whisper.SAMPLING_BEAM_SEARCH,
		WHISPER_GREEDY_SAMPLING: whisper.SAMPLING_GREEDY,
	}
)

type WhisperConfig struct {
	Modelpath        string
	Threads          int
	SamplingStrategy WhisperSampling
	ASRCallback      func(string)
}

type WhisperRecognition struct {
	context *whisper.Context
	params  whisper.Params

	asrCallback func(command string)

	opusData [][]byte
}

func NewWhisper(cfg WhisperConfig) (*WhisperRecognition, error) {
	ctx := whisper.Whisper_init(cfg.Modelpath)
	if ctx == nil {
		return nil, fmt.Errorf("failed to create new Whisper context")
	}

	params := ctx.Whisper_full_default_params(whisperSamplingStrategies[cfg.SamplingStrategy])
	params.SetThreads(cfg.Threads)
	params.SetPrintProgress(false)

	return &WhisperRecognition{
		context:     ctx,
		params:      params,
		asrCallback: cfg.ASRCallback,
		opusData:    make([][]byte, 0),
	}, nil
}

func (r *WhisperRecognition) FeedOpusData(opusData []byte) error {
	isSilence := true
	for _, b := range opusData {
		if b != 0 {
			isSilence = false
			break
		}
	}

	// if not silence add the sound data to buffer
	if !isSilence {
		r.opusData = append(r.opusData, opusData)
		return nil
	}

	if len(r.opusData) == 0 {
		return nil
	}

	pcm := make([]float32, 0)

	for _, data := range r.opusData {
		pcmChunk, err := codec.ConvertOpusToPCMFloat32(16000.0, 1, data)
		if err != nil {
			return fmt.Errorf("while converting data to PCM: %v", err)
		}

		pcm = append(pcm, pcmChunk...)
	}

	startTime := time.Now()

	if err := r.context.Whisper_full(r.params, pcm, nil, nil); err != nil {
		return fmt.Errorf("while full processing Whisper pipeline: %v", err)
	}

	duration := time.Since(startTime)

	for i := 0; i < r.context.Whisper_full_n_segments(); i++ {
		text := r.context.Whisper_full_get_segment_text(i)
		r.asrCallback(fmt.Sprintf("%s: %s", duration, text))
	}

	r.opusData = make([][]byte, 0)

	return nil
}
