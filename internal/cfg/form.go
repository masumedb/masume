package cfg

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/masumedb/masume/internal/core"
	"github.com/masumedb/masume/internal/tunnel"
)

// FormField is one field of the connection form.
type FormField struct {
	Key   string
	Label string
	Value string
	// The allowed values for a choice field.
	Choices []string
}

// buildBlankProfile returns the profile a new connection starts from.
func buildBlankProfile() Profile {
	return Profile{
		Name: "new-connection", Engine: core.DefaultEngine, Host: "127.0.0.1",
		Port: core.ResolveDefaultPort(core.DefaultEngine), Auth: AuthPassword,
		Environment: EnvironmentDev, AccessMode: AccessWrite, Autocommit: true,
		ConfirmWrites: ConfirmOff, WritePlan: PlanOff, UndoRows: DefaultUndoRows,
		CommandTimeout: DefaultCommandTimeout,
		PageSize:       DefaultPageSize, Keepalive: DefaultKeepalive,
	}
}

// resolveDatabaseLabel returns the database or file label for the engine.
func resolveDatabaseLabel(engine core.Engine) string {
	if core.OpensFile(engine) {
		return "file"
	}
	return "database"
}

// The ssh toggle shows the tunnel fields when it is on, and the tls toggle shows the
// certificate fields.
const (
	sshToggleKey = "ssh"
	tlsToggleKey = "tls"
	toggleOff    = "off"
	toggleOn     = "on"
)

// The ssh toggle shows these fields.
var sshFields = map[string]bool{
	"sshHost": true, "sshPort": true, "sshUser": true, "sshKey": true,
	"sshKeyPassphraseEnv": true, "sshPasswordEnv": true, "sshKnownHosts": true,
}

// tlsFields are the certificate fields the tls toggle shows.
var tlsFields = map[string]bool{
	"sslRootCert": true, "sslCert": true, "sslKey": true,
}

// filePathFields are the fields of a file path, whatever the engine is.
var filePathFields = map[string]bool{
	"sslRootCert": true, "sslCert": true, "sslKey": true,
	"sshKey": true, "sshKnownHosts": true,
}

// TakesFilePath is true for a field that holds the path of a file the user can pick. The
// database of a file engine is such a path, and of a server it is a name.
func TakesFilePath(fields []FormField, key string) bool {
	if filePathFields[key] {
		return true
	}
	if key != "database" {
		return false
	}
	engine, known := core.FindEngine(ReadField(fields, "engine"))
	return known && core.OpensFile(engine)
}

// sslModeDefault is the sslmode choice that keeps the default of the engine.
const sslModeDefault = "default"

// listSSLModeChoices returns the sslmode values the form steps through, the engine default
// first.
func listSSLModeChoices() []string {
	names := []string{sslModeDefault}
	for _, mode := range core.SSLModes {
		names = append(names, string(mode))
	}
	return names
}

// describeConfirmValue returns the confirmation as the form holds it. A profile without one
// takes the default of its environment, as a connection does.
func describeConfirmValue(profile Profile) string {
	if profile.ConfirmWrites == "" {
		return string(resolveDefaultConfirmWrites(profile.Environment))
	}
	return string(profile.ConfirmWrites)
}

// describeSSLModeValue returns the mode as the form holds it. An unset mode is the default
// of the engine.
func describeSSLModeValue(mode core.SSLMode) string {
	if mode == core.SSLUnset {
		return sslModeDefault
	}
	return string(mode)
}

// describeToggle returns the value of a toggle field.
func describeToggle(on bool) string {
	if on {
		return toggleOn
	}
	return toggleOff
}

// resolveFormSSHPort returns the ssh port the form shows.
func resolveFormSSHPort(profile Profile) int {
	if profile.SSHPort > 0 {
		return profile.SSHPort
	}
	return tunnel.DefaultPort
}

// serverFields are the fields for the address of a server and the user.
var serverFields = map[string]bool{
	"host": true, "port": true, "user": true, "auth": true,
	"passwordEnv": true, "passwordCommand": true, "sslMode": true,
	"secret": true, "secretRef": true, tlsToggleKey: true,
	"sslRootCert": true, "sslCert": true, "sslKey": true,
}

// passwordFields are the visible fields for each password source. Prompt and keyring modes have no source fields.
var passwordFields = map[AuthMode]map[string]bool{
	AuthPassword: {"passwordEnv": true},
	AuthCommand:  {"passwordCommand": true},
	AuthPrompt:   {},
	AuthKeyring:  {},
	AuthSecret:   {"secret": true, "secretRef": true},
}

var everyPasswordField = map[string]bool{
	"passwordEnv": true, "passwordCommand": true, "secret": true, "secretRef": true,
}

// ReadField returns the value of one field.
func ReadField(fields []FormField, key string) string {
	for _, field := range fields {
		if field.Key == key {
			return field.Value
		}
	}
	return ""
}

func listEngineNames() []string {
	names := make([]string, 0, len(core.Engines))
	for _, engine := range core.Engines {
		names = append(names, string(engine))
	}
	return names
}

func listModeNames[T ~string](allowed []T) []string {
	names := make([]string, 0, len(allowed))
	for _, held := range allowed {
		names = append(names, string(held))
	}
	return names
}

// BuildFormFields returns profile fields and the configured secret store choices.
func BuildFormFields(profile Profile, editing bool, secretStoreNames []string) []FormField {
	source := profile
	if !editing {
		source = buildBlankProfile()
	}
	return []FormField{
		{Key: "name", Label: "name", Value: source.Name},
		{Key: "engine", Label: "engine", Value: string(source.Engine), Choices: listEngineNames()},
		{Key: "host", Label: "host", Value: source.Host},
		{Key: "port", Label: "port", Value: strconv.Itoa(source.Port)},
		{Key: "database", Label: resolveDatabaseLabel(source.Engine), Value: source.Database},
		{Key: "user", Label: "user", Value: source.User},
		{Key: "auth", Label: "auth", Value: string(source.Auth), Choices: listModeNames(AuthModes)},
		{Key: "passwordEnv", Label: "password env", Value: source.PasswordEnv},
		{Key: "passwordCommand", Label: "password command", Value: source.PasswordCommand},
		{
			Key: "secret", Label: "secret store", Value: source.Secret,
			Choices: secretStoreNames,
		},
		{Key: "secretRef", Label: "secret ref", Value: source.SecretRef},
		{
			Key: "environment", Label: "environment", Value: string(source.Environment),
			Choices: listModeNames(Environments),
		},
		{
			Key: "accessMode", Label: "mode", Value: string(source.AccessMode),
			Choices: listModeNames(AccessModes),
		},
		{
			Key: "sslMode", Label: "sslmode",
			Value:   describeSSLModeValue(source.SSLMode),
			Choices: listSSLModeChoices(),
		},
		{
			Key: tlsToggleKey, Label: "tls files",
			Value:   describeToggle(source.BuildSSLFiles().HasFiles()),
			Choices: []string{toggleOff, toggleOn},
		},
		{Key: "sslRootCert", Label: "ssl root cert", Value: source.SSLRootCert},
		{Key: "sslCert", Label: "ssl cert", Value: source.SSLCert},
		{Key: "sslKey", Label: "ssl key", Value: source.SSLKey},
		{
			Key: sshToggleKey, Label: "ssh tunnel",
			Value:   describeToggle(source.OpensTunnel()),
			Choices: []string{toggleOff, toggleOn},
		},
		{Key: "sshHost", Label: "ssh host", Value: source.SSHHost},
		{Key: "sshPort", Label: "ssh port", Value: strconv.Itoa(resolveFormSSHPort(source))},
		{Key: "sshUser", Label: "ssh user", Value: source.SSHUser},
		{Key: "sshKey", Label: "ssh key", Value: source.SSHKey},
		{
			Key: "sshKeyPassphraseEnv", Label: "ssh passphrase env",
			Value: source.SSHKeyPassphraseEnv,
		},
		{Key: "sshPasswordEnv", Label: "ssh password env", Value: source.SSHPasswordEnv},
		{Key: "sshKnownHosts", Label: "ssh known hosts", Value: source.SSHKnownHosts},
		{
			Key: "confirmWrites", Label: "confirm", Value: describeConfirmValue(source),
			Choices: listModeNames(ConfirmModes),
		},
		{Key: "description", Label: "description", Value: source.Description},
		{Key: "aiInstructions", Label: "ai instructions", Value: source.AiInstructions},
	}
}

// FindShownFields filters fields by engine and password source. Hidden field values remain unchanged.
func FindShownFields(fields []FormField) []FormField {
	engine, known := core.FindEngine(ReadField(fields, "engine"))
	if known && core.OpensFile(engine) {
		kept := make([]FormField, 0, len(fields))
		for _, field := range fields {
			if serverFields[field.Key] || sshFields[field.Key] ||
				field.Key == sshToggleKey {
				continue
			}
			kept = append(kept, field)
		}
		return kept
	}

	auth := AuthPassword
	for _, mode := range AuthModes {
		if string(mode) == ReadField(fields, "auth") {
			auth = mode
		}
	}
	read := passwordFields[auth]

	opensTunnel := ReadField(fields, sshToggleKey) == toggleOn
	sendsFiles := ReadField(fields, tlsToggleKey) == toggleOn
	kept := make([]FormField, 0, len(fields))
	for _, field := range fields {
		if everyPasswordField[field.Key] && !read[field.Key] {
			continue
		}
		if sshFields[field.Key] && !opensTunnel {
			continue
		}
		if tlsFields[field.Key] && !sendsFiles {
			continue
		}
		kept = append(kept, field)
	}
	return kept
}

// FormError is a form value that cannot be used to open a connection.
type FormError struct{ Reason string }

func (err FormError) Error() string { return err.Reason }

// findChoice returns the value of the field, or the fallback if the field has none.
func findChoice[T ~string](allowed []T, written string, fallback T) T {
	if found, known := core.FindAllowed(allowed, written); known {
		return found
	}
	return fallback
}

// BuildProfileFromFields validates form values and updates the profile. Settings absent from the form remain unchanged.
func BuildProfileFromFields(fields []FormField, source Profile, editing bool) (Profile, error) {
	built := source
	if !editing {
		built = buildBlankProfile()
	}

	read := func(key string) string { return strings.TrimSpace(ReadField(fields, key)) }
	engine := core.DefaultEngine
	if named, known := core.FindEngine(read("engine")); known {
		engine = named
	}
	opensFile := core.OpensFile(engine)

	built.Name = read("name")
	built.Engine = engine
	built.Host = read("host")
	built.Database = read("database")
	built.User = read("user")
	built.Auth = findChoice(AuthModes, read("auth"), AuthPassword)
	built.Environment = findChoice(Environments, read("environment"), EnvironmentDev)
	built.AccessMode = findChoice(AccessModes, read("accessMode"), AccessWrite)
	built.ConfirmWrites = findChoice(ConfirmModes, read("confirmWrites"), ConfirmOff)
	// Existing passwords from URLs or containers remain in memory only.
	built.PasswordEnv = read("passwordEnv")
	built.PasswordCommand = read("passwordCommand")
	built.Secret = read("secret")
	built.SecretRef = read("secretRef")
	built.Description = read("description")
	built.AiInstructions = read("aiInstructions")

	// A file engine uses no port, so the form does not show one.
	built.Port = core.ResolveDefaultPort(engine)
	if !opensFile {
		port, err := strconv.Atoi(read("port"))
		if err != nil || port <= 0 {
			return Profile{}, FormError{Reason: "port must be a positive integer"}
		}
		built.Port = port
	}

	written := read("sslMode")
	built.SSLMode = core.SSLUnset
	if written != "" && written != sslModeDefault {
		mode, known := core.FindSSLMode(written)
		if !known {
			return Profile{}, FormError{
				Reason: "sslmode must be one of " + core.SSLModeNames(),
			}
		}
		built.SSLMode = mode
	}

	built, filesErr := applyFormSSLFiles(built, read)
	if filesErr != nil {
		return Profile{}, filesErr
	}

	built, tunnelErr := applyFormTunnel(built, read)
	if tunnelErr != nil {
		return Profile{}, tunnelErr
	}

	if built.Name == "" {
		return Profile{}, FormError{Reason: "the profile name is missing"}
	}
	if built.Database == "" && core.NeedsDatabase(engine) {
		if opensFile {
			return Profile{}, FormError{Reason: "the database file path is missing"}
		}
		return Profile{}, FormError{Reason: "the database name is missing"}
	}
	if !opensFile && built.Host == "" {
		return Profile{}, FormError{Reason: "the host is missing"}
	}
	if built.UsesSocket() && !core.TakesSocket(engine) {
		return Profile{}, FormError{
			Reason: string(engine) + " does not connect over a unix socket",
		}
	}
	if core.NeedsUser(engine) && built.User == "" {
		return Profile{}, FormError{Reason: "the user is missing"}
	}
	if built.Auth == AuthSecret && (built.Secret == "" || built.SecretRef == "") {
		return Profile{}, FormError{
			Reason: "auth = secret requires a secret store and reference",
		}
	}
	if built.Auth == AuthCommand && built.PasswordCommand == "" {
		return Profile{}, FormError{
			Reason: "auth = command requires a password command",
		}
	}
	return built, nil
}

// applyFormSSLFiles writes the certificate fields into the profile. An off toggle clears
// every path.
func applyFormSSLFiles(built Profile, read func(string) string) (Profile, error) {
	built.SSLRootCert, built.SSLCert, built.SSLKey = "", "", ""
	if read(tlsToggleKey) != toggleOn {
		return built, nil
	}
	built.SSLRootCert = read("sslRootCert")
	built.SSLCert = read("sslCert")
	built.SSLKey = read("sslKey")
	if reason := core.FindSSLFilesProblem(built.BuildSSLFiles()); reason != "" {
		return Profile{}, FormError{Reason: reason}
	}
	return built, nil
}

// applyFormTunnel writes the tunnel fields into the profile. An off toggle clears every
// ssh setting.
func applyFormTunnel(built Profile, read func(string) string) (Profile, error) {
	built.SSHHost, built.SSHPort, built.SSHUser, built.SSHKey = "", 0, "", ""
	built.SSHKeyPassphraseEnv, built.SSHPasswordEnv, built.SSHKnownHosts = "", "", ""
	if read(sshToggleKey) != toggleOn {
		return built, nil
	}

	port := tunnel.DefaultPort
	if written := read("sshPort"); written != "" {
		held, err := strconv.Atoi(written)
		if err != nil || held <= 0 {
			return Profile{}, FormError{Reason: "the ssh port must be a positive integer"}
		}
		port = held
	}
	built.SSHHost, built.SSHPort, built.SSHUser = read("sshHost"), port, read("sshUser")
	built.SSHKey = read("sshKey")
	built.SSHKeyPassphraseEnv = read("sshKeyPassphraseEnv")
	built.SSHPasswordEnv = read("sshPasswordEnv")
	built.SSHKnownHosts = read("sshKnownHosts")

	if built.SSHHost == "" {
		return Profile{}, FormError{Reason: "the ssh host is missing"}
	}
	if built.SSHUser == "" {
		return Profile{}, FormError{Reason: "the ssh user is missing"}
	}
	return built, nil
}

// ConnectionURL is a parsed connection URL without a password.
type ConnectionURL struct {
	Engine   core.Engine
	Host     string
	Port     int
	Database string
	User     string
	SSLMode  string
	SSLFiles core.SSLFiles
}

// urlSchemes are the engines for supported URL schemes and aliases.
var urlSchemes = func() map[string]core.Engine {
	schemes := map[string]core.Engine{}
	for _, info := range core.ListEngineInfo() {
		for _, scheme := range info.URLSchemes {
			schemes[scheme] = info.Engine
		}
	}
	return schemes
}()

// sslKeys are the query keys a URL can use for the SSL setting.
var sslKeys = []string{"sslmode", "ssl-mode", "sslMode"}

// sslRootCertKeys, sslCertKeys and sslKeyKeys are the query keys a URL can use for the
// certificate files.
var (
	sslRootCertKeys = []string{"sslrootcert", "ssl-root-cert", "sslRootCert"}
	sslCertKeys     = []string{"sslcert", "ssl-cert", "sslCert"}
	sslKeyKeys      = []string{"sslkey", "ssl-key", "sslKey"}
)

// readURLQuery returns the first of these query keys the URL holds a value for.
func readURLQuery(parsed *url.URL, keys []string) string {
	query := parsed.Query()
	for _, key := range keys {
		if written := query.Get(key); written != "" {
			return written
		}
	}
	return ""
}

// readURLSSLFiles returns the certificate files of a URL query.
func readURLSSLFiles(parsed *url.URL) core.SSLFiles {
	return core.SSLFiles{
		RootCert: readURLQuery(parsed, sslRootCertKeys),
		Cert:     readURLQuery(parsed, sslCertKeys),
		Key:      readURLQuery(parsed, sslKeyKeys),
	}
}

// tlsSchemes are the schemes that request TLS by their name, with the mode of each one. A
// Redis client reads `rediss://` as a TLS connection that verifies the certificate, so a
// URL without a mode must not fall back to an unencrypted connection.
var tlsSchemes = map[string]core.SSLMode{"rediss": core.SSLVerifyFull}

// ParseConnectionURL requires a supported scheme and host. A database is required only for an
// engine that connects to one. The returned fields omit the password.
func ParseConnectionURL(text string) (ConnectionURL, bool) {
	trimmed := strings.TrimSpace(text)
	if !strings.Contains(trimmed, "://") {
		return ConnectionURL{}, false
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return ConnectionURL{}, false
	}

	engine, known := urlSchemes[strings.ToLower(parsed.Scheme)]
	if !known {
		return ConnectionURL{}, false
	}

	// Connection hosts use IPv6 addresses without brackets.
	host := strings.Trim(parsed.Hostname(), "[]")
	database := strings.TrimPrefix(parsed.Path, "/")
	if host == "" || (database == "" && core.NeedsDatabase(engine)) {
		return ConnectionURL{}, false
	}

	port := core.ResolveDefaultPort(engine)
	if written := parsed.Port(); written != "" {
		held, portErr := strconv.Atoi(written)
		if portErr != nil || held <= 0 {
			return ConnectionURL{}, false
		}
		port = held
	}

	sslMode := readURLQuery(parsed, sslKeys)
	user := ""
	if parsed.User != nil {
		user = parsed.User.Username()
	}
	return ConnectionURL{
		Engine: engine, Host: host, Port: port, Database: database,
		User: user, SSLMode: sslMode, SSLFiles: readURLSSLFiles(parsed),
	}, true
}

// writeField sets one value and keeps every other field.
func writeField(fields []FormField, key, value string) []FormField {
	written := make([]FormField, 0, len(fields))
	for _, field := range fields {
		if field.Key == key {
			field.Value = value
		}
		written = append(written, field)
	}
	return written
}

// ApplyConnectionURL fills the form from a pasted connection string. The profile name
// follows the database name only while the user has not typed a name.
func ApplyConnectionURL(fields []FormField, held ConnectionURL) []FormField {
	named := strings.TrimSpace(ReadField(fields, "name"))
	if held.Database != "" && (named == "" || named == buildBlankProfile().Name) {
		named = held.Database
	}

	filled := fields
	for _, written := range [][2]string{
		{"engine", string(held.Engine)}, {"host", held.Host},
		{"port", strconv.Itoa(held.Port)}, {"database", held.Database},
		{"user", held.User}, {"sslMode", describeURLSSLMode(held.SSLMode)},
		{"name", named},
		{"sslRootCert", held.SSLFiles.RootCert}, {"sslCert", held.SSLFiles.Cert},
		{"sslKey", held.SSLFiles.Key},
		{tlsToggleKey, describeToggle(held.SSLFiles.HasFiles())},
	} {
		filled = writeField(filled, written[0], written[1])
	}
	return filled
}

// describeURLSSLMode returns the mode of a pasted URL as the form holds it. A URL without
// one keeps the default of the engine.
func describeURLSSLMode(written string) string {
	if strings.TrimSpace(written) == "" {
		return sslModeDefault
	}
	return written
}

// ApplyFieldChange updates a field and related fields. A connection URL in the host field fills the form.
func ApplyFieldChange(fields []FormField, key, value string) []FormField {
	if key == "host" {
		if held, parsed := ParseConnectionURL(value); parsed {
			return ApplyConnectionURL(fields, held)
		}
	}
	changed := writeField(fields, key, value)
	if key != "engine" {
		return changed
	}
	return followEngine(fields, changed, value)
}

// followEngine updates the database label and default port. A non-default port remains unchanged.
func followEngine(before, after []FormField, engine string) []FormField {
	wanted, known := core.FindEngine(engine)
	if !known {
		return after
	}
	previous, hadEngine := core.FindEngine(ReadField(before, "engine"))
	followsPort := hadEngine &&
		ReadField(after, "port") == strconv.Itoa(core.ResolveDefaultPort(previous))

	written := make([]FormField, 0, len(after))
	for _, field := range after {
		if field.Key == "database" {
			field.Label = resolveDatabaseLabel(wanted)
		}
		if field.Key == "port" && followsPort {
			field.Value = strconv.Itoa(core.ResolveDefaultPort(wanted))
		}
		written = append(written, field)
	}
	return written
}

// DescribeAuthMode returns the line the form shows for the password source of that name.
func DescribeAuthMode(written string) string {
	mode, known := core.FindAllowed(AuthModes, written)
	if !known {
		return ""
	}
	switch mode {
	case AuthPassword:
		return "the environment variable in password env"
	case AuthCommand:
		return "the first output line of password command"
	case AuthPrompt:
		return "prompt for the password at every connection"
	case AuthKeyring:
		return "the system keyring holds the password"
	case AuthSecret:
		return "the secret store at secret ref"
	}
	return ""
}

// DescribeSSLMode returns the line the form shows for the TLS mode of that name.
func DescribeSSLMode(written string) string {
	if strings.TrimSpace(written) == sslModeDefault {
		return "the default of this engine"
	}
	mode, known := core.FindSSLMode(written)
	if !known {
		return ""
	}
	switch mode {
	case core.SSLDisable:
		return "no TLS"
	case core.SSLAllow, core.SSLPrefer:
		return "TLS if the server offers it, unverified"
	case core.SSLRequire:
		return "TLS, unverified"
	case core.SSLVerifyCa:
		return "TLS, verify the CA without host name"
	case core.SSLVerifyFull:
		return "TLS, verify the CA and the host name"
	}
	return ""
}

// DescribeConfirmWrites returns the line the form shows for the confirmation of that name.
func DescribeConfirmWrites(written string) string {
	mode, known := core.FindAllowed(ConfirmModes, strings.TrimSpace(written))
	if !known {
		return ""
	}
	switch mode {
	case ConfirmOff:
		return "no confirmation"
	case ConfirmDelete:
		return "confirm a delete"
	case ConfirmWrite:
		return "confirm every write"
	case ConfirmAgent:
		return "confirm every write; an agent may answer"
	}
	return ""
}

// pasteHint is the line the host field shows. A label says what every other field holds,
// and only a field of values names what each one does.
const pasteHint = "paste a postgres:// or mysql:// URL into host to fill the form"

// formFieldLines are the fields whose label does not say enough on its own. Each one names
// the value the field takes.
var formFieldLines = map[string]string{
	"passwordEnv":     "environment variable with the password",
	"passwordCommand": "shell command; first line of stdout is the password",
	"secret":          "store name from the config file",
	"secretRef":       "path inside the secret store",
	"environment":     "dev, test or prod; sets the confirm and write plan defaults",
	tlsToggleKey:      "custom TLS certificate files",
	"sslRootCert":     "CA certificate file, PEM",
	"sslCert":         "client certificate file, PEM",
	"sslKey":          "client key file, PEM",
	sshToggleKey:      "connect through an ssh server",
	"aiInstructions":  "context sent to the AI chat with every request",
}

// DescribeFormField returns the line under the fields: what the value under the caret
// does. A field whose label already says it has no line.
func DescribeFormField(fields []FormField, key, value string) string {
	switch key {
	case "auth":
		return DescribeAuthMode(value)
	case "sslMode":
		return DescribeSSLMode(value)
	case "confirmWrites":
		return DescribeConfirmWrites(value)
	case "host":
		return pasteHint
	}
	return formFieldLines[key]
}

// DescribeFormValue returns a value as the form draws it. An empty field shows a
// placeholder.
func DescribeFormValue(field FormField) string {
	if field.Value != "" {
		return field.Value
	}
	if len(field.Choices) > 0 {
		return fmt.Sprintf("one of %s", strings.Join(field.Choices, ", "))
	}
	return ""
}
