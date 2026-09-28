package tui

// TruncateNameForTest exposes the strip's name-shortening for tests.
func TruncateNameForTest(name string, max int) string { return truncateName(name, max) }
