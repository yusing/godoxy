package gphttp

import (
	"mime"
	"net/http"
)

type (
	ContentType       string
	AcceptContentType []ContentType
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
	for _, v := range h["Accept"] {
		ct, _, err := mime.ParseMediaType(v)
		if err != nil {
			continue
		}
		accepts = append(accepts, ContentType(ct))
	}
	return accepts
}

func (ct ContentType) IsHTML() bool {
	return ct == "text/html" || ct == "application/xhtml+xml"
}

func (ct ContentType) IsJSON() bool {
	return ct == "application/json"
}

func (ct ContentType) IsPlainText() bool {
	return ct == "text/plain"
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

func (act AcceptContentType) AcceptPlainText() bool {
	for _, v := range act {
		if v.IsPlainText() || v == "text/*" || v == "*/*" {
			return true
		}
	}
	return false
}
