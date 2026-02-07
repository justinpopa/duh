package ipxe

import (
	"bytes"
	"fmt"
	"text/template"
)

var linuxTmpl = template.Must(template.New("linux").Parse(`#!ipxe
kernel {{.KernelURL}} {{.Cmdline}}
initrd {{.InitrdURL}}
{{- range .OverlayURLs}}
initrd {{.}}
{{- end}}
boot
`))

var wimbootTmpl = template.Must(template.New("wimboot").Parse(`#!ipxe
kernel {{.KernelURL}}
initrd --name BCD {{.ExtraFileURLs.BCD}}
initrd --name boot.sdi {{.ExtraFileURLs.BootSDI}}
initrd --name boot.wim {{.ExtraFileURLs.BootWIM}}
boot
`))

var esxiTmpl = template.Must(template.New("esxi").Parse(`#!ipxe
kernel {{.KernelURL}} -c {{.ExtraFileURLs.BootCfg}} {{.Cmdline}}
boot
`))

var isoTmpl = template.Must(template.New("iso").Parse(`#!ipxe
kernel {{.KernelURL}} iso raw
initrd {{.ExtraFileURLs.BootISO}}
boot
`))

// ExtraFileURLs holds pre-signed URLs for boot-type-specific extra files.
type ExtraFileURLs struct {
	BCD     string // wimboot: BCD file
	BootSDI string // wimboot: boot.sdi
	BootWIM string // wimboot: boot.wim
	BootCfg string // esxi: boot.cfg
	BootISO string // iso: boot.iso
}

type ScriptParams struct {
	KernelURL     string
	InitrdURL     string
	Cmdline       string
	MAC           string
	Hostname      string
	OverlayURLs   []string
	ExtraFileURLs ExtraFileURLs
}

func RenderBootScript(bootType string, params ScriptParams, ipxeScript string) (string, error) {
	var tmpl *template.Template

	switch bootType {
	case "wimboot":
		tmpl = wimbootTmpl
	case "esxi":
		tmpl = esxiTmpl
	case "iso":
		tmpl = isoTmpl
	case "custom":
		if ipxeScript == "" {
			return ExitScript(), nil
		}
		t, err := template.New("custom").Parse(ipxeScript)
		if err != nil {
			return "", fmt.Errorf("parse custom iPXE script: %w", err)
		}
		tmpl = t
	default: // linux
		tmpl = linuxTmpl
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, params); err != nil {
		return "", fmt.Errorf("render %s boot script: %w", bootType, err)
	}
	return buf.String(), nil
}

func WrapWithConfirmation(script, hostname, mac string) string {
	label := mac
	if hostname != "" {
		label = hostname + " (" + mac + ")"
	}
	return `#!ipxe

menu Confirm Reimage: ` + label + `
item --gap
item --gap This system is flagged for reimage.
item --gap Proceeding will ERASE ALL DATA on this machine.
item --gap
item confirm Proceed with reimage
item cancel  Cancel and boot normally
choose --default cancel --timeout 30000 selected && goto ${selected} || goto cancel

:cancel
echo Cancelled. Booting from local disk...
exit

:confirm
` + stripShebang(script)
}

func stripShebang(script string) string {
	if len(script) > 7 && script[:7] == "#!ipxe\n" {
		return script[7:]
	}
	if len(script) > 8 && script[:8] == "#!ipxe\r\n" {
		return script[8:]
	}
	return script
}

func ExitScript() string {
	return "#!ipxe\nexit\n"
}
