package clipboard

import (
	"context"
	"errors"
	"fmt"
	"time"

	"golang.design/x/clipboard"
)

// requestTimeout bounds every OS clipboard call so a hung clipboard backend
// cannot wedge an HTTP request indefinitely.
const requestTimeout = 3 * time.Second

// Init inicializa el subsistema de clipboard. Debe llamarse una vez al arrancar.
func Init() error {
	return clipboard.Init()
}

// GetText devuelve el texto actual del clipboard del SO.
// Un clipboard sin texto devuelve cadena vacía, no un error.
func GetText() (string, error) {
	data, err := readWithTimeout(clipboard.FmtText)
	if errors.Is(err, clipboard.ErrNoData) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// SetText escribe texto al clipboard del SO.
func SetText(text string) error {
	return writeWithTimeout(clipboard.FmtText, []byte(text))
}

// GetImage devuelve la imagen del clipboard como PNG (bytes).
// Devuelve nil si no hay imagen en el clipboard.
func GetImage() ([]byte, error) {
	data, err := readWithTimeout(clipboard.FmtImage)
	if errors.Is(err, clipboard.ErrNoData) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return data, nil
}

// SetImage escribe una imagen (PNG/JPEG bytes) al clipboard del SO.
func SetImage(data []byte) error {
	if len(data) == 0 {
		return fmt.Errorf("empty image data")
	}
	return writeWithTimeout(clipboard.FmtImage, data)
}

func readWithTimeout(format clipboard.Format) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	return clipboard.Read(ctx, format)
}

func writeWithTimeout(format clipboard.Format, data []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	// Write puts data on the clipboard before returning; the channel only
	// fires when a later external change replaces it, so it must not be awaited.
	_, err := clipboard.Write(ctx, format, data)
	return err
}
