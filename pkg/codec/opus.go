package codec

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"time"
)

const (
	frameLength = time.Duration(20) * time.Millisecond
)

func StreamDCAData(ctx context.Context, dca io.Reader, opusChan chan<- []byte, positionCallback func(position time.Duration)) error {
	var opuslen int16
	framesSent := 0

	for {

		// Read opus frame length from dca file.
		err := binary.Read(dca, binary.LittleEndian, &opuslen)

		// If this is the end of the file, just return.
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			return nil
		}

		if err != nil {
			return fmt.Errorf("while reading length from DCA: %w", err)
		}

		// Read encoded pcm from dca file.
		inBuf := make([]byte, opuslen)
		err = binary.Read(dca, binary.LittleEndian, &inBuf)

		// Should not be any end of file errors
		if err != nil {
			return fmt.Errorf("while reading PCM from DCA: %w", err)
		}

		// Append encoded pcm data to the buffer.
		select {
		case <-ctx.Done():
			return nil
		case opusChan <- inBuf:
			framesSent += 1
			go func() {
				if positionCallback != nil && framesSent%50 == 0 {
					positionCallback(time.Duration(framesSent) * frameLength)
				}
			}()
			continue
		}
	}
}
