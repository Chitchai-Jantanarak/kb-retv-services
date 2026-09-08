package config

// OAuth holds the client credentials Go needs to refresh tenant-owned Gmail
// OAuth tokens against Google. Laravel drives the consent (authorization
// code) flow and stores the resulting refresh token per channel_accounts row;
// Go only ever exchanges a refresh token for a short-lived access token.
type OAuth struct {
	GoogleClientID     string
	GoogleClientSecret string
}
