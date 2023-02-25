package utils

import (
	"mvdan.cc/sh/v3/syntax"
)

// Path contains the default path to the os-release file.
var osReleasePath = "/etc/os-release"

var Release OSRelease

type OSRelease struct {
	ID              string
	IDLike          string
	PrettyName      string
	VersionID       string
	VersionCodename string
	UbuntuCodename  string
	Variant         string
	VariantID       string
}

func (osRelease *OSRelease) mapVariables(key string, data interface{}) {
	switch key {
	case "ID":
		osRelease.ID = data.(string)
	case "ID_LIKE":
		osRelease.IDLike = data.(string)
	case "VERSION_ID":
		osRelease.VersionID = data.(string)
	case "VERSION_CODENAME":
		osRelease.VersionCodename = data.(string)
	case "UBUNTU_CODENAME":
		osRelease.UbuntuCodename = data.(string)
	case "VARIANT":
		osRelease.Variant = data.(string)
	case "VARIANT_ID":
		osRelease.VariantID = data.(string)
	default:
	}
}

func getOSReleaseFile() (*syntax.File, error) {
	file, err := Open(osReleasePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	osReleaseParser := syntax.NewParser(syntax.Variant(syntax.LangBash))
	osReleaseSyntax, err := osReleaseParser.Parse(file, osReleasePath)

	if err != nil {
		return nil, err
	}

	return osReleaseSyntax, err
}

// Parse parses the os-release file pointing to by Path.
// The fields are saved into the Release global variable.
func (osRelease *OSRelease) parseOSRelease(osReleaseSyntax *syntax.File) error {
	var err error

	syntax.Walk(osReleaseSyntax, func(node syntax.Node) bool {
		switch nodeType := node.(type) {
		case *syntax.Assign:
			osRelease.mapVariables(nodeType.Name.Value, StringifyAssign(nodeType))
		}

		return true
	})

	return err
}

func (osRelease *OSRelease) ParseOSRelease() error {
	osReleaseSyntax, err := getOSReleaseFile()

	if err != nil {
		return err
	}

	err = osRelease.parseOSRelease(osReleaseSyntax)
	if err != nil {
		return err
	}

	return err
}
