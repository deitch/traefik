package provider

import (
	"bytes"
	"io/ioutil"
	"strings"
	"text/template"
	"unicode"

	"github.com/BurntSushi/toml"
	"github.com/Masterminds/sprig"
	"github.com/containous/traefik/autogen/gentemplates"
	"github.com/containous/traefik/log"
	"github.com/containous/traefik/safe"
	"github.com/containous/traefik/types"
)

// Provider defines methods of a provider.
type Provider interface {
	// Provide allows the provider to provide configurations to traefik
	// using the given configuration channel.
	Provide(configurationChan chan<- types.ConfigMessage, pool *safe.Pool, constraints types.Constraints) error
}

// BaseProvider should be inherited by providers
type BaseProvider struct {
	Watch                     bool              `description:"Watch provider" export:"true"`
	Filename                  string            `description:"Override default configuration template. For advanced users :)" export:"true"`
	Constraints               types.Constraints `description:"Filter services by constraint, matching with Traefik tags." export:"true"`
	Trace                     bool              `description:"Display additional provider logs (if available)." export:"true"`
	DebugLogGeneratedTemplate bool              `description:"Enable debug logging of generated configuration template." export:"true"`
}

// MatchConstraints must match with EVERY single constraint
// returns first constraint that do not match or nil
func (p *BaseProvider) MatchConstraints(tags []string) (bool, *types.Constraint) {
	// if there is no tags and no constraints, filtering is disabled
	if len(tags) == 0 && len(p.Constraints) == 0 {
		return true, nil
	}

	for _, constraint := range p.Constraints {
		// xor: if ok and constraint.MustMatch are equal, then no tag is currently matching with the constraint
		if ok := constraint.MatchConstraintWithAtLeastOneTag(tags); ok != constraint.MustMatch {
			return false, constraint
		}
	}

	// If no constraint or every constraints matching
	return true, nil
}

// GetConfiguration return the provider configuration using templating
func (p *BaseProvider) GetConfiguration(defaultTemplateFile string, funcMap template.FuncMap, templateObjects interface{}) (*types.Configuration, error) {
	configuration := new(types.Configuration)

	var defaultFuncMap = sprig.TxtFuncMap()
	// tolower is deprecated in favor of sprig's lower function
	defaultFuncMap["tolower"] = strings.ToLower
	defaultFuncMap["normalize"] = Normalize
	defaultFuncMap["split"] = split
	for funcID, funcElement := range funcMap {
		defaultFuncMap[funcID] = funcElement
	}

	tmpl := template.New(p.Filename).Funcs(defaultFuncMap)

	tmplContent, err := p.getTemplateContent(defaultTemplateFile)
	if err != nil {
		return nil, err
	}

	_, err = tmpl.Parse(tmplContent)
	if err != nil {
		return nil, err
	}

	var buffer bytes.Buffer
	err = tmpl.Execute(&buffer, templateObjects)
	if err != nil {
		return nil, err
	}

	var renderedTemplate = buffer.String()
	if p.DebugLogGeneratedTemplate {
		log.Debugf("Rendering results of %s:\n%s", defaultTemplateFile, renderedTemplate)
	}
	if _, err := toml.Decode(renderedTemplate, configuration); err != nil {
		return nil, err
	}
	return configuration, nil
}

func (p *BaseProvider) getTemplateContent(defaultTemplateFile string) (string, error) {
	if len(p.Filename) > 0 {
		buf, err := ioutil.ReadFile(p.Filename)
		if err != nil {
			return "", err
		}
		return string(buf), nil
	}

	if strings.HasSuffix(defaultTemplateFile, ".tmpl") {
		buf, err := gentemplates.Asset(defaultTemplateFile)
		if err != nil {
			return "", err
		}
		return string(buf), nil
	}

	return defaultTemplateFile, nil
}

func split(sep, s string) []string {
	return strings.Split(s, sep)
}

// Normalize transform a string that work with the rest of traefik
// Replace '.' with '-' in quoted keys because of this issue https://github.com/BurntSushi/toml/issues/78
func Normalize(name string) string {
	fargs := func(c rune) bool {
		return !unicode.IsLetter(c) && !unicode.IsNumber(c)
	}
	// get function
	return strings.Join(strings.FieldsFunc(name, fargs), "-")
}

// ReverseStringSlice invert the order of the given slice of string
func ReverseStringSlice(slice *[]string) {
	for i, j := 0, len(*slice)-1; i < j; i, j = i+1, j-1 {
		(*slice)[i], (*slice)[j] = (*slice)[j], (*slice)[i]
	}
}
