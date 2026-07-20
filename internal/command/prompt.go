package command

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"
)

func promptForConfirmation(input io.Reader, output io.Writer, question string) (bool, error) {
	reader := bufio.NewReader(input)
	for {
		if _, err := fmt.Fprintf(output, "%s [y/N] ", question); err != nil {
			return false, err
		}

		response, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return false, err
		}

		switch strings.ToLower(strings.TrimSpace(response)) {
		case "y", "yes":
			return true, nil
		case "", "n", "no":
			if !errors.Is(err, io.EOF) || response != "" {
				return false, nil
			}
		}

		if errors.Is(err, io.EOF) {
			return false, io.ErrUnexpectedEOF
		}
		if _, err := fmt.Fprintln(output, "Please answer y or n."); err != nil {
			return false, err
		}
	}
}
