package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/masumedb/masume/internal/cfg"
	"github.com/masumedb/masume/internal/db/engines"
	"github.com/masumedb/masume/internal/headless"
)

const diffUsage = `masume diff - compare the schemas of two connections

usage:
  masume diff SOURCE TARGET

  SOURCE, TARGET         a supported URL, keyword connection string, existing SQLite file,
                         or -p NAME for a user or project profile
  -s, --schema NAME      the source schema; without it, the default schema of the connection
      --target-schema NAME
                         the target schema; without it, the source schema or the default schema
      --sql              write the ALTER script that changes the source schema to the target
  -h, --help             print this help and exit

exit codes:
  0 no differences
  1 differences found
  2 argument, password, connection or catalog read failure

Each line is a table, column, index or constraint: + only in the target, - only in the
source, ~ in both with a different definition. The ALTER script is written for the
PostgreSQL and MySQL families, with both connections of one family.`

// diffSide is one connection of a compare: a target or a profile name.
type diffSide struct {
	target      string
	profileName string
}

type diffInvocation struct {
	sides        []diffSide
	sourceSchema string
	targetSchema string
	script       bool
	help         bool
}

var diffShortFlagNames = map[string]string{"-p": "--profile", "-s": "--schema"}

func parseDiffArguments(argv []string) (diffInvocation, error) {
	held := diffInvocation{}
	for at := 0; at < len(argv); at++ {
		argument := expandShortFlag(argv[at], diffShortFlagNames)
		var value string
		var err error

		switch {
		case argument == "--help" || argument == "-h":
			held.help = true
		case argument == "--profile" || argument == "-p":
			if value, at, err = readFlagValue(argv, at, argument); err != nil {
				return diffInvocation{}, err
			}
			held.sides = append(held.sides, diffSide{profileName: value})
		case strings.HasPrefix(argument, "--profile="):
			if value, err = readFlagText(argument, "--profile="); err != nil {
				return diffInvocation{}, err
			}
			held.sides = append(held.sides, diffSide{profileName: value})
		case argument == "--schema" || argument == "-s":
			if held.sourceSchema, at, err = readFlagValue(argv, at, argument); err != nil {
				return diffInvocation{}, err
			}
		case strings.HasPrefix(argument, "--schema="):
			if held.sourceSchema, err = readFlagText(argument, "--schema="); err != nil {
				return diffInvocation{}, err
			}
		case argument == "--target-schema":
			if held.targetSchema, at, err = readFlagValue(argv, at, argument); err != nil {
				return diffInvocation{}, err
			}
		case strings.HasPrefix(argument, "--target-schema="):
			if held.targetSchema, err = readFlagText(argument, "--target-schema="); err != nil {
				return diffInvocation{}, err
			}
		case argument == "--sql":
			held.script = true
		case strings.HasPrefix(argument, "-"):
			return diffInvocation{}, failArgument("unknown masume diff option: " + argument)
		default:
			held.sides = append(held.sides, diffSide{target: argument})
		}
	}
	if !held.help && len(held.sides) != 2 {
		return diffInvocation{}, failArgument(fmt.Sprintf(
			"masume diff requires two connections, a source and a target; received %d",
			len(held.sides)))
	}
	return held, nil
}

// resolveDiffSide returns the profile and the password of one side.
func resolveDiffSide(side diffSide, profiles []cfg.Profile) (cfg.Profile, string, int) {
	_, start, err := resolveStartProfile(
		invocation{target: side.target, profileName: side.profileName},
		profiles, func(string) string { return "" })
	if err != nil {
		fmt.Fprintln(os.Stderr, "masume: "+err.Error())
		return cfg.Profile{}, "", 2
	}
	password, err := cfg.ResolveProfilePassword(*start)
	if err != nil {
		fmt.Fprintln(os.Stderr, "masume: "+err.Error())
		return cfg.Profile{}, "", headless.CodeConnection
	}
	return *start, password, headless.CodeOK
}

// runDiffCommand runs `masume diff` and returns the exit code of the process.
func runDiffCommand(argv []string) int {
	held, err := parseDiffArguments(argv)
	if err != nil {
		fmt.Fprintln(os.Stderr, "masume: "+err.Error())
		fmt.Fprintln(os.Stderr, "run masume diff --help for usage")
		return 2
	}
	if held.help {
		fmt.Println(diffUsage)
		return 0
	}

	loaded := loadCommandConfig()
	source, sourcePassword, code := resolveDiffSide(held.sides[0], loaded.Profiles)
	if code != headless.CodeOK {
		return code
	}
	target, targetPassword, code := resolveDiffSide(held.sides[1], loaded.Profiles)
	if code != headless.CodeOK {
		return code
	}
	return headless.RunDiff(context.Background(), engines.CreateAdapters(),
		headless.DiffOptions{
			Options: headless.Options{
				Profile: source, Password: sourcePassword, Out: os.Stdout, Err: os.Stderr,
			},
			Target: headless.Options{
				Profile: target, Password: targetPassword, Out: os.Stdout, Err: os.Stderr,
			},
			SourceSchema: held.sourceSchema, TargetSchema: held.targetSchema,
			Script: held.script,
		})
}
