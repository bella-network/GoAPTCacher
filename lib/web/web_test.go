package web

import (
	"testing"
)

// TestGetTemplate tests the GetTemplate function.
func TestGetTemplate(t *testing.T) {
	// Reset the tpl variable before each test
	tpl = nil

	// Call GetTemplate and check for errors
	tmpl, err := GetTemplate()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Check if the returned template is not nil
	if tmpl == nil {
		t.Fatalf("expected a template, got nil")
	}

	// Check if the returned template is the same as the global tpl variable
	if tmpl != tpl {
		t.Fatalf("expected the returned template to be the same as the global tpl variable")
	}

	// Call GetTemplate again and check if it returns the same template
	tmpl2, err := GetTemplate()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if tmpl2 != tmpl {
		t.Fatalf("expected the same template to be returned, got different templates")
	}
}

// TestGetTemplateError tests the GetTemplate function when there is an error parsing the template.
func TestGetTemplateError(t *testing.T) {
	// Backup the original MainPage
	originalMainPage := MainPage
	defer func() { MainPage = originalMainPage }()

	// Set MainPage to an invalid template
	MainPage = []byte("{{ invalid template }}")

	// Reset the tpl variable before each test
	tpl = nil

	// Call GetTemplate and check for errors
	_, err := GetTemplate()
	if err == nil {
		t.Fatalf("expected an error, got nil")
	}
}
