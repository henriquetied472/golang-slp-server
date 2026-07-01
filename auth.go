package main

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"slices"

	"github.com/fsnotify/fsnotify"
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
		return nil
	}
	defer ret.Body.Close()
	raw, _ := io.ReadAll(ret.Body)
	var res HttpAuthResponse
	json.Unmarshal(raw, &res)
	if res.Error != "" {
		slog.Error("HttpAuthProvider: an error occurred when authenticating: " + res.Error)
		return nil
	}
	return must(hex.DecodeString(res.PasswordSHA1))
}

type JsonAuthProvider struct {
	Table map[string]any
	Filename string
}

func NewJsonAuthProvider(filename string) *JsonAuthProvider {
	return &JsonAuthProvider{
		Filename: filename,
	} 
}

func (auth *JsonAuthProvider) Verify(username string, challenge []byte, response []byte) bool {
	sha1 := auth.GetUserPasswordSHA1(username)
	return slices.Equal(SHA1(append(sha1, challenge...)), response)
}

func (auth *JsonAuthProvider) Read() {
	auth.Table = make(map[string]any)
	raw, err := os.Open(auth.Filename)
	if err != nil {
		slog.Error("JsonAuthProvider: couldn't read the file: " + err.Error())
		return
	}
	defer raw.Close()
	json.Unmarshal(must(io.ReadAll(raw)), &auth.Table)
	delete(auth.Table, "$schema")
}

func (auth *JsonAuthProvider) GetUserPasswordSHA1(username string) []byte {
	pw, ok := auth.Table[username]
	if !ok {
		slog.Error("JsonAuthProvider: no such user called \"" + username + "\"")
		return nil
	}

	var ret []byte
	switch pw := pw.(type) {
	case string:
		ret = SHA1(pw)
	default:
		pass, ok := pw.(map[string]string)["sha1"]
		if !ok {
			slog.Error("JsonAuthProvider: couldn't parse json")
			return nil
		}
		ret1, err := hex.DecodeString(pass)
		if err != nil {
			slog.Error("JsonAuthProvider: couldn't parse password sha1 hex: " + err.Error())
		}
		ret = ret1
	}
	return ret
}

func (auth *JsonAuthProvider) Run(ctx context.Context) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		slog.Error("JsonAuthProvider: couldn't start file watcher: " + err.Error())
		return
	}
	
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case err, ok := <- w.Errors:
				if !ok {
					return
				}

				slog.Error("JsonAuthProvider: watcher error: " + err.Error())
			case event, ok := <- w.Events:
				if !ok {
					return
				}

				if event.Has(fsnotify.Write) {
					auth.Read()
				}
			}
		}
	}()

	err = w.Add(auth.Filename)
	if err != nil {
		slog.Error("JsonAuthProvider: error when trying to add a file to the watcher: " + err.Error())
		w.Close()
		return
	}
}

