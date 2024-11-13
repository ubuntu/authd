//go:build !withgdmmodel

// Package gdm is the package for the GDM pam module handing.
package gdm

// PamExtensionCustomJSON is the gdm PAM extension for passing string values.
const PamExtensionCustomJSON = ""

// IsPamExtensionSupported returns if the provided extension is supported.
func IsPamExtensionSupported(extension string) bool {
	return false
}
