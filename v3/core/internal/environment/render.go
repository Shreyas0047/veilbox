package environment

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"text/template"
)

// renderFromTemplate renders an RPM-owned template file into a
// user-side file, writing atomically. It creates parent directories
// but never overwrites existing content — callers guard first-touch.
func (e *Engine) renderFromTemplate(tmplPath, outPath string, data any) error {
	raw, err := os.ReadFile(tmplPath)
	if err != nil {
		return fmt.Errorf("read template %s: %w", tmplPath, err)
	}
	tmpl, err := template.New(filepath.Base(tmplPath)).Parse(string(raw))
	if err != nil {
		return fmt.Errorf("parse template %s: %w", tmplPath, err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return fmt.Errorf("render template %s: %w", tmplPath, err)
	}
	dir := filepath.Dir(outPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create directory %s: %w", dir, err)
	}
	tmp := outPath + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, outPath); err != nil {
		return fmt.Errorf("commit %s: %w", outPath, err)
	}
	return nil
}
