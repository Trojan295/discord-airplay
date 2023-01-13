package codec

import (
	"encoding/binary"
	"fmt"
	"io"
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
