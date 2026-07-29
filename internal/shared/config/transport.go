package config

type Laravel struct {
	BaseURL          string
	HostHeader       string
	WebhookSecret    string
	TicketPath       string
	Timeout          int
	JWTSecret        string
	JWTPublicKeyPath string
	JWKSURL          string
}

type Line struct {
	ChannelSecret      string
	ChannelAccessToken string
}
