//go:build !windows

package windows

import "fmt"

type UserControl struct{}

func NewUserControl() *UserControl {
	return &UserControl{}
}

func (u *UserControl) SetPassword(username, password string) error {
	return fmt.Errorf("user control only supported on Windows")
}

func (u *UserControl) DisconnectUserSession(username string) error {
	return fmt.Errorf("user control only supported on Windows")
}
