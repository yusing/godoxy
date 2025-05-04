package gphttp

import (
	"mime"
	"net/http"
	"strings"
)

type (
	ContentType       string
	AcceptContentType []ContentType
)

const (
	ContentTypeJSON         = ContentType("application/json")
	ContentTypeTextPlain    = ContentType("text/plain")
	ContentTypeTextHTML     = ContentType("text/html")
	ContentTypeTextMarkdown = ContentType("text/markdown")
	ContentTypeTextXML      = ContentType("text/xml")
	ContentTypeXHTML        = ContentType("application/xhtml+xml")
)

func GetContentType(h http.Header) ContentType {
	ct := h.Get("Content-Type")
	if ct == "" {
		return ""
	}
	ct, _, err := mime.ParseMediaType(ct)
	if err != nil {
		return ""
	}
	return ContentType(ct)
}

func GetAccept(h http.Header) AcceptContentType {
	var accepts []ContentType
	acceptHeader := h["Accept"]
	if len(acceptHeader) == 1 {
		acceptHeader = strings.Split(acceptHeader[0], ",")
	}
	for _, v := range acceptHeader {
		ct, _, err := mime.ParseMediaType(v)
		if err != nil {
			continue
		}
		accepts = append(accepts, ContentType(ct))
	}
	if len(accepts) == 0 {
		return []ContentType{"*/*"}
	}
	return accepts
}

func (ct ContentType) IsHTML() bool {
	return ct == ContentTypeTextHTML || ct == ContentTypeXHTML
}

func (ct ContentType) IsJSON() bool {
	return ct == ContentTypeJSON
}

func (ct ContentType) IsPlainText() bool {
	return ct == ContentTypeTextPlain
}

func (act AcceptContentType) IsEmpty() bool {
	return len(act) == 0
}

func (act AcceptContentType) AcceptHTML() bool {
	for _, v := range act {
		if v.IsHTML() || v == "text/*" || v == "*/*" {
			return true
		}
	}
	return false
}

func (act AcceptContentType) AcceptJSON() bool {
	for _, v := range act {
		if v.IsJSON() || v == "*/*" {
			return true
		}
	}
	return false
}

func (act AcceptContentType) AcceptMarkdown() bool {
	for _, v := range act {
		if v == ContentTypeTextMarkdown || v == "*/*" {
			return true
		}
	}
	return false
}

func (act AcceptContentType) AcceptPlainText() bool {
	for _, v := range act {
		if v.IsPlainText() || v == "text/*" || v == "*/*" {
			return true
		}
	}
	return false
}
