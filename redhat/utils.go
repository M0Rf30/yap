package redhat

func ConvertSection(section string) string { //nolint:cyclop
	var converted string

	switch section {
	case "admin":
		converted = "Applications/System"
	case "localization":
		converted = "Development/Languages"
	case "mail":
		converted = Communications
	case "comm":
		converted = Communications
	case "math":
		converted = "Applications/Productivity"
	case "database":
		converted = "Applications/Databases"
	case "misc":
		converted = "Applications/System"
	case "debug":
		converted = "Development/Debuggers"
	case "net":
		converted = Internet
	case "news":
		converted = "Applications/Publishing"
	case "devel":
		converted = Tools
	case "doc":
		converted = "Documentation"
	case "editors":
		converted = "Applications/Editors"
	case "electronics":
		converted = Engineering
	case "embedded":
		converted = Engineering
	case "fonts":
		converted = "Interface/Desktops"
	case "games":
		converted = "Amusements/Games"
	case "science":
		converted = Engineering
	case "shells":
		converted = "System Environment/Shells"
	case "sound":
		converted = Multimedia
	case "graphics":
		converted = Multimedia
	case "text":
		converted = "Applications/Text"
	case "httpd":
		converted = Internet
	case "vcs":
		converted = Tools
	case "interpreters":
		converted = Tools
	case "video":
		converted = Multimedia
	case "web":
		converted = Internet
	case "kernel":
		converted = "System Environment/Kernel"
	case "x11":
		converted = "User Interface/X"
	case "libdevel":
		converted = "Development/Libraries"
	case "libs":
		converted = "System Environment/Libraries"
	default:
		converted = section
	}

	return converted
}
