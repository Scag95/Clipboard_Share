package clipboard

import (
	"context"
	"fmt"

	"golang.design/x/clipboard"
)

// Init inicializa el subsistema de clipboard. Debe llamarse una vez al arrancar.
func Init() error {
	return clipboard.Init()
}

// GetText devuelve el texto actual del clipboard del SO.
func GetText() (string, error) {
	data, err := clipboard.Read(context.Background(), clipboard.FmtText)
	if err != nil {
		return "", err
	}
	if data == nil {
		return "", nil
	}
	return string(data), nil
}

// SetText escribe texto al clipboard del SO.
func SetText(text string) error {
	_, err := clipboard.Write(context.Background(), clipboard.FmtText, []byte(text))
	if err != nil {
		return err
	}
	return nil
}

// GetImage devuelve la imagen del clipboard como PNG (bytes). Devuelve nil si no hay imagen.
func GetImage() ([]byte, error) {
	data, err := clipboard.Read(context.Background(), clipboard.FmtImage)
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, nil
	}
	return data, nil
}

// SetImage escribe una imagen (PNG/JPEG bytes) al clipboard del SO.
func SetImage(data []byte) error {
	if len(data) == 0 {
		return fmt.Errorf("empty image data")
	}
	_, err := clipboard.Write(context.Background(), clipboard.FmtImage, data)
	if err != nil {
		return err
	}
	return nil
}
