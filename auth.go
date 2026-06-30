package main

import (
	"crypto/sha1"
)

func SHA1[T string | []byte](data T) []byte {
	raw := []byte(data)
	
	sum := sha1.Sum(raw)
	return sum[:]
}

type AuthProvider interface {
	Verify(username string, chalenge []byte, response []byte) bool
}

