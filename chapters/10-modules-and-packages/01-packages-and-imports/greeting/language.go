package greeting

// Language identifies a supported greeting language.
type Language uint8

const (
	English Language = iota
	Spanish
)

const defaultPrefix = "Hello"

// prefixFor is available to every file in package greeting, but not its callers.
func prefixFor(language Language) (string, bool) {
	switch language {
	case English:
		return defaultPrefix, true
	case Spanish:
		return "Hola", true
	default:
		return "", false
	}
}
