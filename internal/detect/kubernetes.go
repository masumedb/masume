package detect

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"github.com/masumedb/masume/internal/cfg"
	"github.com/masumedb/masume/internal/core"
)

// kubeTool is the Kubernetes command line the current context is read with.
const kubeTool = "kubectl"

// kubeRequestTimeout is the time limit kubectl gives each request to the cluster.
const kubeRequestTimeout = "--request-timeout=10s"

// pod is the supported subset of a pod in the answer of `kubectl get pods -o json`.
type pod struct {
	Metadata struct {
		Name      string
		Namespace string
	}
	Spec struct {
		Containers []podContainer
	}
	Status struct {
		Phase string
	}
}

type podContainer struct {
	Name  string
	Image string
	Ports []struct {
		ContainerPort int
		Protocol      string
	}
	Env []struct {
		Name      string
		Value     string
		ValueFrom *struct {
			SecretKeyRef    *keyReference
			ConfigMapKeyRef *keyReference
		}
	}
	EnvFrom []struct {
		Prefix       string
		SecretRef    *struct{ Name string }
		ConfigMapRef *struct{ Name string }
	}
}

// keyReference is one key of a Secret or a ConfigMap.
type keyReference struct {
	Name string
	Key  string
}

// kubeSource reads the cluster of the current context: the pods, and the Secrets and
// ConfigMaps their environment reads.
type kubeSource struct {
	context string
	read    func(arguments ...string) ([]byte, error)
	// The data of each Secret and ConfigMap already read, by namespace, kind and name.
	held map[string]map[string]string
}

// readKeyData returns the decoded data of one Secret or ConfigMap, or nothing where it
// cannot be read.
func (source *kubeSource) readKeyData(namespace, kind, name string) map[string]string {
	cacheKey := namespace + "/" + kind + "/" + name
	if data, found := source.held[cacheKey]; found {
		return data
	}
	data := map[string]string{}
	source.held[cacheKey] = data
	written, err := source.read("get", kind, name, "--namespace", namespace, "--output", "json")
	if err != nil {
		return data
	}
	var object struct{ Data map[string]string }
	if json.Unmarshal(written, &object) != nil {
		return data
	}
	for key, value := range object.Data {
		if kind == "secret" {
			decoded, decodeErr := base64.StdEncoding.DecodeString(value)
			if decodeErr != nil {
				continue
			}
			value = string(decoded)
		}
		data[key] = value
	}
	return data
}

// readPodEnvironment returns the environment of a container, with the values its Secrets
// and ConfigMaps hold, and the Secret key of each variable that reads one. A variable in
// env replaces a variable of the same name from envFrom.
func (source *kubeSource) readPodEnvironment(
	namespace string, container podContainer,
) (map[string]string, map[string]keyReference) {
	environment := map[string]string{}
	secrets := map[string]keyReference{}
	for _, imported := range container.EnvFrom {
		switch {
		case imported.SecretRef != nil:
			name := imported.SecretRef.Name
			for key, value := range source.readKeyData(namespace, "secret", name) {
				environment[imported.Prefix+key] = value
				secrets[imported.Prefix+key] = keyReference{Name: name, Key: key}
			}
		case imported.ConfigMapRef != nil:
			for key, value := range source.readKeyData(namespace, "configmap", imported.ConfigMapRef.Name) {
				environment[imported.Prefix+key] = value
			}
		}
	}
	for _, variable := range container.Env {
		switch {
		case variable.ValueFrom == nil:
			environment[variable.Name] = variable.Value
			delete(secrets, variable.Name)
		case variable.ValueFrom.SecretKeyRef != nil:
			reference := *variable.ValueFrom.SecretKeyRef
			secrets[variable.Name] = reference
			environment[variable.Name] = source.readKeyData(namespace, "secret", reference.Name)[reference.Key]
		case variable.ValueFrom.ConfigMapKeyRef != nil:
			reference := *variable.ValueFrom.ConfigMapKeyRef
			delete(secrets, variable.Name)
			environment[variable.Name] = source.readKeyData(namespace, "configmap", reference.Name)[reference.Key]
		}
	}
	return environment, secrets
}

// resolvePodPort returns the port of the container that serves the database: the default
// port of the engine where the container declares it or declares none, and the first port
// it declares otherwise.
func resolvePodPort(container podContainer, engine core.Engine) int {
	fallback := core.ResolveDefaultPort(engine)
	for _, port := range container.Ports {
		if port.ContainerPort == fallback {
			return fallback
		}
	}
	for _, port := range container.Ports {
		if port.Protocol == "" || port.Protocol == "TCP" {
			return port.ContainerPort
		}
	}
	return fallback
}

// plainArgument matches an argument that every shell reads as it is written.
var plainArgument = regexp.MustCompile(`^[A-Za-z0-9._:/@=-]+$`)

// quoteArgument quotes a value as one argument of the shell that runs a profile command.
func quoteArgument(value string) string {
	if plainArgument.MatchString(value) {
		return value
	}
	if runtime.GOOS == "windows" {
		return `"` + strings.ReplaceAll(value, `"`, `\"`) + `"`
	}
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

// buildPasswordCommand returns the command that prints the value of one Secret key.
func (source *kubeSource) buildPasswordCommand(namespace string, reference keyReference) string {
	template := fmt.Sprintf(`{{index .data %q | base64decode}}`, reference.Key)
	return strings.Join([]string{
		kubeTool, "get", "secret", quoteArgument(reference.Name),
		"--namespace", quoteArgument(namespace), "--context", quoteArgument(source.context),
		"--output", quoteArgument("go-template=" + template),
	}, " ")
}

// findFreePort returns a port of this machine that nothing listens on now.
func findFreePort() (int, error) {
	listener, err := net.Listen("tcp", localHost+":0")
	if err != nil {
		return 0, err
	}
	defer func() { _ = listener.Close() }()
	return listener.Addr().(*net.TCPAddr).Port, nil
}

// buildPodProfile returns a profile that reaches one database container of a pod through
// `kubectl port-forward` on a free port of this machine.
func (source *kubeSource) buildPodProfile(held pod, container podContainer) (cfg.Profile, bool, error) {
	engine, known := findImageEngine(container.Image)
	if !known {
		return cfg.Profile{}, false, nil
	}
	localPort, err := findFreePort()
	if err != nil {
		return cfg.Profile{}, false, err
	}
	namespace, name := held.Metadata.Namespace, held.Metadata.Name
	remotePort := resolvePodPort(container, engine)

	profile := cfg.Profile{
		Name: name, Engine: engine, Host: localHost, Port: localPort,
		Auth: cfg.AuthPassword, Environment: cfg.EnvironmentDev,
		AccessMode: cfg.AccessWrite, SSLMode: resolveContainerSSLMode(engine),
		Autocommit: true, ConfirmWrites: cfg.ConfirmOff, WritePlan: cfg.PlanOff,
		UndoRows:       cfg.DefaultUndoRows,
		CommandTimeout: cfg.DefaultCommandTimeout, PageSize: cfg.DefaultPageSize,
		Keepalive: cfg.DefaultKeepalive,
		Command: strings.Join([]string{
			kubeTool, "port-forward", "--context", quoteArgument(source.context),
			"--namespace", quoteArgument(namespace), quoteArgument("pod/" + name),
			strconv.Itoa(localPort) + ":" + strconv.Itoa(remotePort),
		}, " "),
		WaitForPort: localPort,
		Description: fmt.Sprintf("%s in pod %s/%s, context %s",
			container.Image, namespace, name, source.context),
	}

	environment, secrets := source.readPodEnvironment(namespace, container)
	applyEnvironment(&profile, environment)
	for variable, reference := range secrets {
		if profile.Password != "" && environment[variable] == profile.Password {
			profile.Auth = cfg.AuthCommand
			profile.PasswordCommand = source.buildPasswordCommand(namespace, reference)
			profile.Password = ""
			break
		}
	}
	return profile, true, nil
}

// BuildProfilesFromPods returns one connection per database container in the answer of
// `kubectl get pods --output json`. Only running pods are read.
func BuildProfilesFromPods(
	written []byte, kubeContext string, read func(arguments ...string) ([]byte, error),
) ([]cfg.Profile, error) {
	var list struct{ Items []pod }
	if err := json.Unmarshal(written, &list); err != nil {
		return nil, fmt.Errorf("invalid pod list JSON: %w", err)
	}
	source := &kubeSource{context: kubeContext, read: read, held: map[string]map[string]string{}}
	found := []cfg.Profile{}
	for _, held := range list.Items {
		if held.Status.Phase != "Running" {
			continue
		}
		for _, container := range held.Spec.Containers {
			profile, holds, err := source.buildPodProfile(held, container)
			if err != nil {
				return nil, err
			}
			if !holds {
				continue
			}
			profile.Name = cfg.ResolveUniqueProfileName(found, profile.Name)
			found = append(found, profile)
		}
	}
	return found, nil
}

// errNoKubeContext marks a machine without kubectl or without a current context.
var errNoKubeContext = errors.New("no kubectl context")

// BuildKubernetesProfiles returns one connection per database that runs in a pod of the
// current kubectl context.
func BuildKubernetesProfiles() ([]cfg.Profile, error) {
	if _, err := exec.LookPath(kubeTool); err != nil {
		return nil, errNoKubeContext
	}
	ctx, stop := context.WithTimeout(context.Background(), scanTimeout)
	defer stop()

	current, err := runTool(ctx, kubeTool, "config", "current-context")
	if err != nil || strings.TrimSpace(string(current)) == "" {
		return nil, errNoKubeContext
	}
	kubeContext := strings.TrimSpace(string(current))
	read := func(arguments ...string) ([]byte, error) {
		return runTool(ctx, kubeTool, append(arguments, "--context", kubeContext, kubeRequestTimeout)...)
	}
	written, err := read("get", "pods", "--all-namespaces", "--output", "json")
	if err != nil {
		return nil, err
	}
	return BuildProfilesFromPods(written, kubeContext, read)
}

// BuildDetectedProfiles returns the databases of the containers of this machine and of the
// pods of the current kubectl context. A source that fails is reported only where no
// source found a database.
func BuildDetectedProfiles() ([]cfg.Profile, error) {
	containers, containerErr := BuildContainerProfiles()
	pods, podErr := BuildKubernetesProfiles()
	found := append(containers, pods...)
	if len(found) > 0 {
		return found, nil
	}
	problems := []string{}
	if containerErr != nil {
		problems = append(problems, containerErr.Error())
	}
	if podErr != nil && !errors.Is(podErr, errNoKubeContext) {
		problems = append(problems, podErr.Error())
	}
	if len(problems) > 0 {
		return nil, errors.New(strings.Join(problems, "; "))
	}
	return found, nil
}
