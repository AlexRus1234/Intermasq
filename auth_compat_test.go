package main

import "intermask/internal/auth"

var DBPath = auth.DBPath

func setUsers(values map[string]string) {
	auth.ClearUsers()
	for name, hash := range values {
		auth.SetUser(name, hash)
	}
}
