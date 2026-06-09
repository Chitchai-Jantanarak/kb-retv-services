package helpers

import (
	"bytes"
	"compress/zlib"
	"io"

	"github.com/my/app/pkg/utils"
)

func Compress(input []byte) []byte {
	var buf bytes.Buffer
	writer := zlib.NewWriter(&buf)
	_, err := writer.Write(input)
	if err != nil {
		return nil
	}
	writer.Close()
	return buf.Bytes()
}

func Decompress(data []byte) *string {
	buf := bytes.NewReader(data)
	reader, err := zlib.NewReader(buf)
	if err != nil {
		return nil
	}
	defer reader.Close()
	decoded, err := io.ReadAll(reader)
	if err != nil {
		return nil
	}
	return utils.Ptr(string(decoded))
}
