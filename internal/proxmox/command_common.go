package proxmox

import (
	"fmt"
	"strings"
)

// checkValidInput checks if the input contains invalid characters.
//
// The characters are: & | $ ; ' " ` $( ${ < >
// These characters are used in the command line to escape the input or to expand variables.
// We need to check if the input contains these characters and return an error if it does.
// This is to prevent command injection.
func checkValidInput(input string) error {
	if strings.ContainsAny(input, "&|$;'\"`<>") {
		return fmt.Errorf("input contains invalid characters: %q", input)
	}
	if strings.Contains(input, "$(") {
		return fmt.Errorf("input contains $(: %q", input)
	}
	if strings.Contains(input, "${") {
		return fmt.Errorf("input contains ${: %q", input)
	}
	return nil
}

func formatTail(files []string, limit int) (string, error) {
	for _, file := range files {
		if err := checkValidInput(file); err != nil {
			return "", err
		}
	}
	var command strings.Builder
	command.WriteString("tail -f -q --retry ")
	for _, file := range files {
		fmt.Fprintf(&command, " %q ", file)
	}
	if limit > 0 {
		fmt.Fprintf(&command, " -n %d", limit)
	}
	return command.String(), nil
}

func formatJournalctl(services []string, limit int) (string, error) {
	for _, service := range services {
		if err := checkValidInput(service); err != nil {
			return "", err
		}
	}
	var command strings.Builder
	command.WriteString("journalctl -f")
	for _, service := range services {
		fmt.Fprintf(&command, " -u %q ", service)
	}
	if limit > 0 {
		fmt.Fprintf(&command, " -n %d", limit)
	}
	return command.String(), nil
}
