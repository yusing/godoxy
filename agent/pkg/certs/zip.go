package certs

import (
	"archive/zip"
	"bytes"
	"io"
	"path/filepath"

	"github.com/yusing/go-proxy/internal/common"
)

func writeFile(zipWriter *zip.Writer, name string, data []byte) error {
	w, err := zipWriter.CreateHeader(&zip.FileHeader{
		Name:   name,
		Method: zip.Store,
	})
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

func readFile(f *zip.File) ([]byte, error) {
	r, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}

func ZipCert(ca, crt, key []byte) ([]byte, error) {
	data := bytes.NewBuffer(make([]byte, 0, 6144))
	zipWriter := zip.NewWriter(data)
	defer zipWriter.Close()

	if err := writeFile(zipWriter, "ca.pem", ca); err != nil {
		return nil, err
	}
	if err := writeFile(zipWriter, "cert.pem", crt); err != nil {
		return nil, err
	}
	if err := writeFile(zipWriter, "key.pem", key); err != nil {
		return nil, err
	}
	if err := zipWriter.Close(); err != nil {
		return nil, err
	}
	return data.Bytes(), nil
}

func AgentCertsFilename(host string) string {
	return filepath.Join(common.AgentCertsBasePath, host+".zip")
}

func ExtractCert(data []byte) (ca, crt, key []byte, err error) {
	zipReader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, nil, nil, err
	}
	for _, file := range zipReader.File {
		switch file.Name {
		case "ca.pem":
			ca, err = readFile(file)
		case "cert.pem":
			crt, err = readFile(file)
		case "key.pem":
			key, err = readFile(file)
		}
		if err != nil {
			return nil, nil, nil, err
		}
	}
	return ca, crt, key, nil
}
