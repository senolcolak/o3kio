// Package scs implements SCS (Sovereign Cloud Stack) standards conformance
// helpers. This file covers SCS-0104-v1 Standard Images: the embedded manifest,
// a validator that operators can hook into image-create paths, and a catalog
// conformance check for an operator-facing report.
package scs

import (
	_ "embed"
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed scs_0104_images.yaml
var scs0104ManifestYAML []byte

// ImageStatus is the SCS-0104 status of an image entry.
type ImageStatus string

const (
	ImageStatusMandatory   ImageStatus = "mandatory"
	ImageStatusRecommended ImageStatus = "recommended"
	ImageStatusOptional    ImageStatus = "optional"
)

// ImageSpec is one entry from the SCS-0104 manifest. Either Name (exact match)
// or NameScheme (regex match) drives identification; Sources lists the URL
// prefixes that an image's image_source property MUST start with.
type ImageSpec struct {
	Name       string      `yaml:"name"`
	NameScheme string      `yaml:"name_scheme"`
	Sources    []string    `yaml:"source"`
	Status     ImageStatus `yaml:"status"`

	nameSchemeRE *regexp.Regexp
}

// matches reports whether the given image name belongs to this spec — by exact
// name match or, if a name_scheme regex is defined, by regex match.
func (s ImageSpec) matches(name string) bool {
	if s.Name != "" && s.Name == name {
		return true
	}
	if s.nameSchemeRE != nil && s.nameSchemeRE.MatchString(name) {
		return true
	}
	return false
}

// sourceOK reports whether the given image_source starts with one of the
// declared prefixes for this spec.
func (s ImageSpec) sourceOK(source string) bool {
	for _, p := range s.Sources {
		if strings.HasPrefix(source, p) {
			return true
		}
	}
	return false
}

// ImageSpecList is the loaded SCS-0104 manifest.
type ImageSpecList []ImageSpec

// Lookup returns the spec whose Name exactly matches the given image name. The
// exact-name lookup is what the manifest test wants; for resolving an image at
// validation time, see findSpec which also honours name_scheme.
func (l ImageSpecList) Lookup(name string) (ImageSpec, bool) {
	for _, s := range l {
		if s.Name == name {
			return s, true
		}
	}
	return ImageSpec{}, false
}

// findSpec resolves an image name to a spec, honouring both exact `name` and
// `name_scheme` regex match. Used by the validator.
func (l ImageSpecList) findSpec(name string) (ImageSpec, bool) {
	for _, s := range l {
		if s.matches(name) {
			return s, true
		}
	}
	return ImageSpec{}, false
}

// CatalogImage is one row in an operator-supplied Glance catalog snapshot,
// passed to CheckCatalog for the conformance report.
type CatalogImage struct {
	Name   string
	Source string
}

// BadSourceFinding flags an image whose name matches an SCS-0104 spec but whose
// image_source does not start with any of the declared prefixes.
type BadSourceFinding struct {
	Name   string
	Source string
}

// CatalogReport is the result of CheckCatalog: which mandatory SCS-0104 images
// the operator is missing, and which present images have non-conformant sources.
type CatalogReport struct {
	MissingMandatory []string
	BadSources       []BadSourceFinding
}

// CheckCatalog runs the SCS-0104 conformance check over an operator-supplied
// catalog: mandatory images that aren't present are reported as missing, and
// images whose name matches an SCS spec but whose source doesn't match any
// declared prefix are reported as bad-source findings.
func (l ImageSpecList) CheckCatalog(catalog []CatalogImage) CatalogReport {
	report := CatalogReport{}

	// Mandatory check: each mandatory spec must have at least one catalog
	// entry whose name matches.
	for _, spec := range l {
		if spec.Status != ImageStatusMandatory {
			continue
		}
		found := false
		for _, img := range catalog {
			if spec.matches(img.Name) {
				found = true
				break
			}
		}
		if !found && spec.Name != "" {
			report.MissingMandatory = append(report.MissingMandatory, spec.Name)
		}
	}

	// Source check: every catalog image with an SCS-known name must have a
	// matching source prefix.
	for _, img := range catalog {
		spec, ok := l.findSpec(img.Name)
		if !ok {
			continue
		}
		if !spec.sourceOK(img.Source) {
			report.BadSources = append(report.BadSources, BadSourceFinding{
				Name:   img.Name,
				Source: img.Source,
			})
		}
	}

	return report
}

var loadedSpecs ImageSpecList

func init() {
	specs, err := loadSpecs(scs0104ManifestYAML)
	if err != nil {
		// The manifest is embedded at compile time; a parse error here is a
		// build-time bug, not a runtime condition.
		panic(fmt.Sprintf("scs: failed to load embedded SCS-0104 manifest: %v", err))
	}
	loadedSpecs = specs
}

func loadSpecs(data []byte) (ImageSpecList, error) {
	var doc struct {
		Images ImageSpecList `yaml:"images"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse SCS-0104 manifest: %w", err)
	}
	for i := range doc.Images {
		// Default unspecified status to "optional" per the spec.
		if doc.Images[i].Status == "" {
			doc.Images[i].Status = ImageStatusOptional
		}
		if scheme := doc.Images[i].NameScheme; scheme != "" {
			re, err := regexp.Compile(scheme)
			if err != nil {
				return nil, fmt.Errorf("compile name_scheme %q: %w", scheme, err)
			}
			doc.Images[i].nameSchemeRE = re
		}
	}
	return doc.Images, nil
}

// ImageSpecs returns the loaded SCS-0104 manifest.
func ImageSpecs() ImageSpecList {
	return loadedSpecs
}

// ValidateImageSource checks an image's name and image_source against the
// SCS-0104 manifest. Unknown names pass through unchanged — operators are free
// to publish their own images under their own names. Known SCS names must have
// a non-empty image_source that starts with one of the declared prefixes;
// otherwise an error is returned that names the image and lists the allowed
// prefixes so the operator gets actionable feedback.
func ValidateImageSource(name, source string) error {
	spec, ok := loadedSpecs.findSpec(name)
	if !ok {
		return nil
	}
	if source == "" {
		return fmt.Errorf("image %q matches SCS-0104 spec but has no image_source; allowed prefixes: %v", name, spec.Sources)
	}
	if !spec.sourceOK(source) {
		return fmt.Errorf("image %q has image_source %q which does not match any SCS-0104 prefix; allowed prefixes: %v", name, source, spec.Sources)
	}
	return nil
}
