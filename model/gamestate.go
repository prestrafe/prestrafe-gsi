package model

type GameState struct {
	Auth     *AuthState     `json:"auth"`
	Map      *MapState      `json:"map"`
	Player   *PlayerState   `json:"player"`
	Provider *ProviderState `json:"provider"`
}

type AuthState struct {
	Token string `json:"token"`
}

type ProviderState struct {
	Name      string `json:"name"`
	AppId     int    `json:"appid"`
	Version   int    `json:"version"`
	SteamId   int64  `json:"steamid,string"`
	Timestamp int64  `json:"timestamp"`
}

type MapState struct {
	Name string `json:"name"`
}

type PlayerState struct {
	SteamId    int64       `json:"steamid,string"`
	Clan       string      `json:"clan"`
	Name       string      `json:"name"`
	MatchStats *MatchStats `json:"match_stats"`
}

type MatchStats struct {
	Kills   int `json:"kills"`
	Assists int `json:"assists"`
	Deaths  int `json:"deaths"`
	Mvps    int `json:"mvps"`
	Score   int `json:"score"`
}
