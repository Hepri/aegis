package port

// UserControl controls user password and session on the client machine
type UserControl interface {
	// SetPassword sets the password for the given username
	SetPassword(username, password string) error

	// DisconnectUserSession disconnects the user's session (WTSDisconnectSession)
	DisconnectUserSession(username string) error
}
