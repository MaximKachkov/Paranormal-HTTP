package model

import (
	"errors"
	"strings"
)

func (input DossierInput) Validate() error {
	switch {
	case strings.TrimSpace(input.Name) == "":
		return errors.New("name is required")
	case strings.TrimSpace(input.Description) == "":
		return errors.New("description is required")
	case input.ThreatLevel < 1 || input.ThreatLevel > 10:
		return errors.New("invalid threat_level")
	case input.Vulnerabilities == nil:
		return errors.New("vulnerabilities is required")
	default:
		return nil
	}
}
