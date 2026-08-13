package mailer

import (
	"bytes"
	"encoding/base64"
	"io"
)

func writeHeader(buf *bytes.Buffer, key, value string) {
	buf.WriteString(key)
	buf.WriteString(": ")
	buf.WriteString(value)
	buf.WriteString("\r\n")
}

func writeBase64(w io.Writer, data []byte) {
	encoded := base64.StdEncoding.EncodeToString(data)
	for len(encoded) > 76 {
		_, _ = io.WriteString(w, encoded[:76]+"\r\n")
		encoded = encoded[76:]
	}
	if len(encoded) > 0 {
		_, _ = io.WriteString(w, encoded+"\r\n")
	}
}
