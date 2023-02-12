package codec

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"

	"gopkg.in/hraban/opus.v2"
)

func ConvertDCAtoOpus(dca io.Reader) ([][]byte, error) {
	buffer := make([][]byte, 0)

	var opuslen int16

	for {
		// Read opus frame length from dca file.
		err := binary.Read(dca, binary.LittleEndian, &opuslen)

		// If this is the end of the file, just return.
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			return buffer, nil
		}

		if err != nil {
			return nil, fmt.Errorf("while reading length from DCA: %w", err)
		}

		// Read encoded pcm from dca file.
		InBuf := make([]byte, opuslen)
		err = binary.Read(dca, binary.LittleEndian, &InBuf)

		// Should not be any end of file errors
		if err != nil {
			return nil, fmt.Errorf("while reading PCM from DCA: %w", err)
		}

		// Append encoded pcm data to the buffer.
		buffer = append(buffer, InBuf)
	}
}

func ConvertOpusToPCM(sampleRate float64, channels int, opusData []byte) ([]byte, error) {
	var frameSizeMs float64 = 20
	frameSize := float64(channels) * frameSizeMs * sampleRate / 1000

	dec, err := opus.NewDecoder(int(sampleRate), channels)
	if err != nil {
		return nil, fmt.Errorf("while creating decoder: %v", err)
	}

	pcmData := make([]int16, int(frameSize))

	if _, err = dec.Decode(opusData, pcmData); err != nil {
		return nil, fmt.Errorf("while creating decoder: %v", err)
	}

	buf := new(bytes.Buffer)
	if err := binary.Write(buf, binary.LittleEndian, pcmData); err != nil {
		return nil, fmt.Errorf("while converting to byte array: %v", err)
	}

	return buf.Bytes(), nil
}

func ConvertOpusToPCMFloat32(sampleRate float64, channels int, opusData []byte) ([]float32, error) {
	var frameSizeMs float64 = 20
	frameSize := float64(channels) * frameSizeMs * sampleRate / 1000

	dec, err := opus.NewDecoder(int(sampleRate), channels)
	if err != nil {
		return nil, fmt.Errorf("while creating decoder: %v", err)
	}

	pcmData := make([]float32, int(frameSize))

	if _, err = dec.DecodeFloat32(opusData, pcmData); err != nil {
		return nil, fmt.Errorf("while decoding opus: %v", err)
	}

	return pcmData, nil
}
