package contactsui

import (
	"errors"
	"fmt"
	"net/mail"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// ErrNoTerminal is returned when there is no terminal to draw on.
var ErrNoTerminal = errors.New("the contacts screens need a terminal")

// OpenTTY opens the controlling terminal, so a screen can draw and read keys
// while stdin and stdout are pipes: `contacts pick` prints its result on
// stdout for the program that ran it.
func OpenTTY() (*os.File, error) {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNoTerminal, err)
	}
	return tty, nil
}

// Run runs model full screen on tty and returns the model it ended with.
func Run(model tea.Model, tty *os.File) (tea.Model, error) {
	p := tea.NewProgram(model, tea.WithInput(tty), tea.WithOutput(tty),
		tea.WithAltScreen(), tea.WithMouseCellMotion())
	final, err := p.Run()
	if err != nil {
		return nil, fmt.Errorf("contacts screen: %w", err)
	}
	return final, nil
}

// splitAddress is the name and lowercased address in entry, which may be a
// bare address or "Name <addr>".
func splitAddress(entry string) (string, string) {
	if a, err := mail.ParseAddress(strings.TrimSpace(entry)); err == nil {
		return a.Name, strings.ToLower(a.Address)
	}
	return "", strings.ToLower(strings.TrimSpace(entry))
}
