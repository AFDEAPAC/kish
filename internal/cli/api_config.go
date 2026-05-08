package cli

import "os"

const (
	kishAPIURLEnv   = "KISH_API_URL"
	kishAPITokenEnv = "KISH_API_TOKEN"
)

// resolveAPIBaseValue returns the API base URL using the shared CLI priority:
// explicit flag first, then KISH_API_URL.
func resolveAPIBaseValue(flagValue string) string {
	if flagValue != "" {
		return flagValue
	}
	return os.Getenv(kishAPIURLEnv)
}

// resolveTokenValue returns the API token using the shared CLI priority:
// explicit flag first, then KISH_API_TOKEN.
func resolveTokenValue(flagValue string) string {
	if flagValue != "" {
		return flagValue
	}
	return os.Getenv(kishAPITokenEnv)
}
