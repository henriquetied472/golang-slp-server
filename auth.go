package main

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
)

func SHA1[T string | []byte](data T) []byte {
	raw := []byte(data)
	
	sum := sha1.Sum(raw)
	return sum[:]
}

type AuthProvider interface {
	Verify(username string, challenge []byte, response []byte) bool
	GetUserPasswordSHA1(username string) []byte
}

type HttpAuthProvider struct {
	Url string
}

type HttpAuthResponse struct {
	Error string `json:"error"`
	PasswordSHA1 string `json:"passwordSHA1"`
}

func NewHttpAuthProvider(url string) *HttpAuthProvider {
	return &HttpAuthProvider{
		Url: url,
	}
}

func (auth *HttpAuthProvider) Verify(username string, challenge []byte, response []byte) bool {
	sha1 := auth.GetUserPasswordSHA1(username)
	return slices.Equal(SHA1(append(sha1, challenge...)), response)
}

func (auth *HttpAuthProvider) GetUserPasswordSHA1(username string) []byte {
	final_url := auth.Url + "?username=" + url.QueryEscape(username)
	ret, err := http.Get(final_url)
	if err != nil {
		slog.Error("HttpAuthProvider: something happened in the request: " + err.Error())
	}
	defer ret.Body.Close()
	raw, _ := io.ReadAll(ret.Body)
	var res HttpAuthResponse
	json.Unmarshal(raw, &res)
	if res.Error != "" {
		slog.Error("HttpAuthProvider: an error occurred when authenticating: " + err.Error())
	}
	return must(hex.DecodeString(res.PasswordSHA1))
}

