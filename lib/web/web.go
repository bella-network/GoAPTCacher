// This package contains the static web files for the GoAPTCacher web interface.
package web

import (
	_ "embed"
	"text/template"
)

//go:embed web/index.html
var MainPage []byte

//go:embed web/style.css
var Style []byte

//go:embed web/favicon.ico
var Favicon []byte

var tpl *template.Template

// GetTemplate returns the template for the given template name.
func GetTemplate() (*template.Template, error) {
	// Check if the template is already loaded
	if tpl != nil {
		return tpl, nil
	}

	// Load the template
	newTemplate := template.New("main")
	newTemplate, err := newTemplate.Parse(string(MainPage))
	if err != nil {
		return nil, err
	}

	tpl = newTemplate
	return tpl, nil
}
