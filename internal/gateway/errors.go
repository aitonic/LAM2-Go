package gateway

import (
	"encoding/json"
	"net/http"
	"strings"
	"unicode"
)

func upstreamErrorMessage(raw []byte, h http.Header) string {
	var body struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
		Message string `json:"message"`
	}
	if json.Unmarshal(raw, &body) != nil {
		return "Upstream returned a non-success status"
	}
	msg := body.Error.Message
	if msg == "" {
		msg = body.Message
	}
	if msg == "" {
		return "Upstream returned a non-success status"
	}
	for _, vs := range h {
		for _, v := range vs {
			for _, secret := range []string{v, strings.TrimPrefix(v, "Bearer ")} {
				if len(secret) >= 4 {
					msg = strings.ReplaceAll(msg, secret, "[redacted]")
				}
			}
		}
	}
	msg = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, msg)
	chars := []rune(msg)
	if len(chars) > 512 {
		msg = string(chars[:512]) + "..."
	}
	return "Upstream: " + msg
}
