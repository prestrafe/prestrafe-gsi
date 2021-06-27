package model

// Data structure sent by server, it is structured this way to economize bandwidth

type ServerState struct {
	ServerInfo ServerInfo   `json:"serverInfo"`
	PlayerInfo []PlayerInfo `json:"playerInfo"`
}

type ServerInfo struct {
	TimeStamp      int    `json:"timestamp"`
	ServerName     string `json:"servername"`
	MapName        string `json:"mapname"`
	TimeoutsCTPrev int    `json:"timeoutsCTprev"`
	TimeoutsTPrev  int    `json:"timeoutsTprev"`
	TimeoutsCT     int    `json:"timeoutsCT"`
	TimeoutsT      int    `json:"timeoutsT"`
	Global         int    `json:"global"`
}

type PlayerInfo struct {
	AuthKey      string  `json:"authkey"`
	SteamId      int64   `json:"steamid,string"`
	Clan         string  `json:"clan"` // SteamID, clan and name already exists in the gsi data, do we need to send this again?
	Name         string  `json:"name"`
	TimeInServer float64 `json:"timeinserver"` // Need a better name
	KZData       KZData  `json:"KZData"`
}

type KZData struct {
	Global      bool    `json:"global"`
	Course      int     `json:"course"`
	Time        float64 `json:"time"`
	Checkpoints int     `json:"checkpoints"`
	Teleports   int     `json:"teleports"`
}

// Stored data structure that will be communicated to bot
type FullPlayerInfo struct {
	TimeStamp      int     `json:"timestamp"`
	AuthKey        string  `json:"authkey"`
	TimeoutsCTPrev int     `json:"timeoutsCTprev"`
	TimeoutsTPrev  int     `json:"timeoutsTprev"`
	TimeoutsCT     int     `json:"timeoutsCT"`
	TimeoutsT      int     `json:"timeoutsT"`
	ServerName     string  `json:"servername"`
	MapName        string  `json:"mapname"`
	ServerGlobal   int     `json:"serverglobal"`
	SteamId        int64   `json:"steamid,string"`
	Clan           string  `json:"clan"`
	Name           string  `json:"name"`
	TimeInServer   float64 `json:"timeinserver"` // Need a better name
	KZData         KZData  `json:"KZData"`
}

func New(sInfo *ServerInfo, pInfo *PlayerInfo) *FullPlayerInfo {
	return &FullPlayerInfo{
		sInfo.TimeStamp,
		pInfo.AuthKey,
		sInfo.TimeoutsCTPrev,
		sInfo.TimeoutsTPrev,
		sInfo.TimeoutsCT,
		sInfo.TimeoutsT,
		sInfo.ServerName,
		sInfo.MapName,
		sInfo.Global,
		pInfo.SteamId,
		pInfo.Clan,
		pInfo.Name,
		pInfo.TimeInServer,
		pInfo.KZData,
	}
}
