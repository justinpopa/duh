package profile

import (
	"bytes"
	"encoding/json"
	"fmt"
	"text/template"
)

type TemplateVars struct {
	MAC         string
	Hostname    string
	IP          string
	SystemID    int64
	ImageID     int64
	ServerURL   string
	ConfigURL   string
	CallbackURL string
	Vars        map[string]string
}

func BuildVars(defaultVarsJSON, systemVarsJSON string) (map[string]string, error) {
	merged := make(map[string]string)

	if defaultVarsJSON != "" && defaultVarsJSON != "{}" {
		if err := json.Unmarshal([]byte(defaultVarsJSON), &merged); err != nil {
			return nil, fmt.Errorf("parse profile default_vars: %w", err)
		}
	}

	if systemVarsJSON != "" && systemVarsJSON != "{}" {
		var overrides map[string]string
		if err := json.Unmarshal([]byte(systemVarsJSON), &overrides); err != nil {
			return nil, fmt.Errorf("parse system vars: %w", err)
		}
		for k, v := range overrides {
			merged[k] = v
		}
	}

	return merged, nil
}

func RenderConfigTemplate(configTemplate string, vars TemplateVars) (string, error) {
	tmpl, err := template.New("config").Parse(configTemplate)
	if err != nil {
		return "", fmt.Errorf("parse config template: %w", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, vars); err != nil {
		return "", fmt.Errorf("render config template: %w", err)
	}
	return buf.String(), nil
}

func RenderKernelParams(kernelParams string, vars TemplateVars) (string, error) {
	if kernelParams == "" {
		return "", nil
	}
	tmpl, err := template.New("kparams").Parse(kernelParams)
	if err != nil {
		return "", fmt.Errorf("parse kernel_params template: %w", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, vars); err != nil {
		return "", fmt.Errorf("render kernel_params template: %w", err)
	}
	return buf.String(), nil
}
