// Package protocol defines the JSON messages exchanged between the server
// and bots over WebSocket. These structs are the source of truth for
// schema/king.schema.json; run `go generate ./protocol` after changing them.
package protocol

import "github.com/cenkalti/king-server/king"

// Message type values for the "type" field.
const (
	TypeHello    = "hello"
	TypeWelcome  = "welcome"
	TypeError    = "error"
	TypeRequest  = "request"
	TypeResponse = "response"
	TypeMatchEnd = "match_end"
)

// Kind is what a Request asks the bot to decide.
type Kind string

const (
	ChooseContract Kind = "choose_contract"
	PlayCard       Kind = "play_card"
)

// Envelope is used to read the "type" field before decoding a message.
type Envelope struct {
	Type string `json:"type"`
}

// Hello is the first message a bot sends after connecting.
type Hello struct {
	Type string `json:"type" jsonschema:"enum=hello"`
	// Name identifies the bot for the whole match. Reconnect with the same name.
	Name string `json:"name" jsonschema:"minLength=1,maxLength=32"`
}

// Welcome confirms a Hello.
type Welcome struct {
	Type string `json:"type" jsonschema:"enum=welcome"`
	Name string `json:"name"`
}

// Error reports a rejected Hello or a protocol error. The server closes the
// connection after sending it.
type Error struct {
	Type    string `json:"type" jsonschema:"enum=error"`
	Message string `json:"message"`
}

// Request asks the bot for one decision. Exactly one of LegalCards or
// LegalContracts is set, matching Kind.
type Request struct {
	Type   string `json:"type" jsonschema:"enum=request"`
	ID     string `json:"id"`
	GameID int    `json:"game_id"`
	Kind   Kind   `json:"kind" jsonschema:"enum=choose_contract,enum=play_card"`
	State  State  `json:"state"`
	// LegalCards is set for play_card.
	LegalCards []king.Card `json:"legal_cards,omitempty"`
	// LegalContracts is set for choose_contract.
	LegalContracts []king.Contract `json:"legal_contracts,omitempty"`
}

// Response answers the Request with the same ID. Set Card for play_card and
// Contract for choose_contract.
type Response struct {
	Type     string         `json:"type" jsonschema:"enum=response"`
	ID       string         `json:"id"`
	Card     *king.Card     `json:"card,omitempty"`
	Contract *king.Contract `json:"contract,omitempty"`
}

// Standing is one bot's line in the final standings.
type Standing struct {
	Rank            int     `json:"rank"`
	Name            string  `json:"name"`
	PlacementPoints float64 `json:"placement_points"`
	TotalPoints     int     `json:"total_points"`
	GamesWon        float64 `json:"games_won"`
	Strikes         int     `json:"strikes"`
}

// MatchEnd is sent to every bot when the match is over.
type MatchEnd struct {
	Type      string     `json:"type" jsonschema:"enum=match_end"`
	Standings []Standing `json:"standings"`
}
