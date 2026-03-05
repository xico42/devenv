package provision

import (
	"bytes"
	"embed"
	"fmt"
	"text/template"
)

//go:embed templates/user-data.yaml.tmpl
var templateFS embed.FS

// Params holds rendering parameters for the cloud-init user-data template.
type Params struct {
	Username         string
	Hostname         string
	TailscaleAuthKey string
}

// RenderUserData renders the cloud-init user-data template with the given params.
func RenderUserData(p Params) (string, error) {
	tmpl, err := template.ParseFS(templateFS, "templates/user-data.yaml.tmpl")
	if err != nil {
		return "", fmt.Errorf("parsing user-data template: %w", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, p); err != nil {
		return "", fmt.Errorf("rendering user-data template: %w", err)
	}
	return buf.String(), nil
}
