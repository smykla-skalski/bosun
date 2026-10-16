package assets

import _ "embed"

//go:embed replies.pl.json
var polishReplies string

// PolishReplies returns the embedded Polish reply catalog.
func PolishReplies() string {
	return polishReplies
}
