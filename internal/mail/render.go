// Package mail renders and delivers calm, multipart Afterwords email.
package mail

import (
	"bytes"
	"embed"
	"fmt"
	htmltemplate "html/template"
	"path/filepath"
	texttemplate "text/template"
)

//go:embed templates/*.html templates/*.txt
var templateFiles embed.FS

// TemplateData is intentionally limited to non-secret email content.
type TemplateData struct {
	OwnerEmail    string
	RecipientName string
	TrusteeName   string
	SwitchName    string
	Deadline      string
	CheckinURL    string
	ReleaseURL    string
	KitURL        string
	DaysOverdue   int
	Test          bool
}

// Render renders matching HTML and plain-text templates.
func Render(name string, data TemplateData) (string, string, error) {
	base := filepath.Base(name)
	if base != name || name == "." || name == "" {
		return "", "", fmt.Errorf("invalid template name")
	}
	htmlBytes, err := templateFiles.ReadFile("templates/" + name + ".html")
	if err != nil {
		return "", "", err
	}
	textBytes, err := templateFiles.ReadFile("templates/" + name + ".txt")
	if err != nil {
		return "", "", err
	}
	htmlTemplate, err := htmltemplate.New(name).Parse(string(htmlBytes))
	if err != nil {
		return "", "", err
	}
	textTemplate, err := texttemplate.New(name).Parse(string(textBytes))
	if err != nil {
		return "", "", err
	}
	var htmlOutput, textOutput bytes.Buffer
	if err := htmlTemplate.Execute(&htmlOutput, data); err != nil {
		return "", "", err
	}
	if err := textTemplate.Execute(&textOutput, data); err != nil {
		return "", "", err
	}
	return htmlOutput.String(), textOutput.String(), nil
}
