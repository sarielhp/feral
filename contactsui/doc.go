// Package contactsui holds the full-screen contacts screens: the picker that
// `contacts pick` runs for other programs, the manager that `contacts tui`
// runs, and the List they share with the compose screen's F5 picker.
//
// It depends on the contacts package and bubbletea only -- never on the mail
// TUI -- so it can be run on its own, embedded in another bubbletea program,
// or, one day, moved into a module of its own.
package contactsui
