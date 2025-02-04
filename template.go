package replace

import (
	"bytes"
	"os"
	"strings"
	"text/template"

	sprig "github.com/Masterminds/sprig"
)

type templateData struct {
	Arg map[string]string
	Env map[string]string
}

func CreateTemplate() *template.Template {
	tmpl := template.New("base")
	tmpl.Funcs(sprig.TxtFuncMap())
	tmpl.Option("missingkey=zero")

	return tmpl
}

func ParseContentAsTemplate(templateContent string, changesets []Changeset) (bytes.Buffer, error) {
	var content bytes.Buffer
	data := generateTemplateData(changesets)
	tmpl, err := CreateTemplate().Parse(templateContent)
	if err != nil {
		return content, err
	}

	err = tmpl.Execute(&content, &data)
	if err != nil {
		return content, err
	}

	return content, nil
}

func generateTemplateData(changesets []Changeset) templateData {
	// init
	var ret templateData
	ret.Arg = make(map[string]string)
	ret.Env = make(map[string]string)

	// add changesets
	for _, changeset := range changesets {
		ret.Arg[changeset.SearchPlain] = changeset.Replace
	}

	// add env variables
	for _, e := range os.Environ() {
		split := strings.SplitN(e, "=", 2)
		envKey, envValue := split[0], split[1]
		ret.Env[envKey] = envValue
	}

	return ret
}
