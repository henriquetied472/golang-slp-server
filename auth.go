package main

type AuthProvider interface {
	Verify(username string, chalenge []byte, response []byte) bool
}