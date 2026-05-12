package main

type HostEntry struct {
	Mac      string `json:"mac"`
	Ip       string `json:"ip"`
	Hostname string `json:"hostname"`
	File     string `json:"file"`
}

// НОВОЕ: Для массового импорта
type BulkHostReq struct {
	Hosts []HostEntry `json:"hosts"`
	File  string      `json:"file"`
}

type LeaseEntry struct {
	Ip       string `json:"ip"`
	Mac      string `json:"mac"`
	Hostname string `json:"hostname"`
}

type AuthReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
}
