package secret

// IsAvailable is true on Windows. Credential Manager is part of the operating system.
var IsAvailable = func() bool { return true }
