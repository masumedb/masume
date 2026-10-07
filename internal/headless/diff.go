package headless

import (
	"context"
	"fmt"

	"github.com/masumedb/masume/internal/core"
	"github.com/masumedb/masume/internal/db"
	"github.com/masumedb/masume/internal/db/engines"
	"github.com/masumedb/masume/internal/schemadiff"
)

// DiffOptions is a headless schema compare request. Options holds the source connection.
type DiffOptions struct {
	Options
	Target       Options
	SourceSchema string
	// Empty uses the source schema where one is named, and the default schema otherwise.
	TargetSchema string
	// True writes the ALTER script that changes the source schema to the target, in place of
	// the report.
	Script bool
}

// The exit codes of `masume diff`.
const (
	CodeSame      = 0
	CodeDifferent = 1
)

// diffSide is one open connection of a compare and the schema read from it.
type diffSide struct {
	session  db.Session
	snapshot schemadiff.Snapshot
}

// RunDiff opens both connections, writes the differences, and returns the exit code.
func RunDiff(ctx context.Context, adapters engines.Adapters, options DiffOptions) int {
	source, closeSource, code := openDiffSide(ctx, adapters, options.Options, options.SourceSchema)
	if code != CodeOK {
		return code
	}
	defer closeSource()
	targetSchema := options.TargetSchema
	if targetSchema == "" {
		targetSchema = options.SourceSchema
	}
	target, closeTarget, code := openDiffSide(ctx, adapters, options.Target, targetSchema)
	if code != CodeOK {
		return code
	}
	defer closeTarget()

	found := schemadiff.Compare(source.snapshot, target.snapshot)
	text := schemadiff.WriteReport(found)
	if options.Script {
		var written bool
		if text, written = writeDiffScript(ctx, options, source, target); !written {
			return CodeConnection
		}
	}
	if _, err := fmt.Fprint(options.Out, text); err != nil {
		options.report("cannot write the report: %s", err)
		return CodeConnection
	}
	if len(found) > 0 {
		return CodeDifferent
	}
	return CodeSame
}

// writeDiffScript returns the ALTER script, or reports why there is none.
func writeDiffScript(
	ctx context.Context, options DiffOptions, source, target diffSide,
) (string, bool) {
	family := core.ResolveEngineInfo(options.Profile.Engine).Family
	if family != core.ResolveEngineInfo(options.Target.Profile.Engine).Family {
		options.report("an ALTER script needs two servers of the same engine family")
		return "", false
	}
	script, err := schemadiff.WriteScript(ctx, schemadiff.ScriptRequest{
		Source: source.snapshot, Target: target.snapshot,
		Family: family, Dialect: source.session.Dialect(), TargetCatalog: target.session,
		SessionSchema: source.session.Describe().DefaultSchema,
	})
	if err != nil {
		options.report("%s", db.DescribeError(err))
		return "", false
	}
	return script.Write(), true
}

// openDiffSide opens one connection and reads one schema. Every failure is exit code 2.
func openDiffSide(
	ctx context.Context, adapters engines.Adapters, options Options, schema string,
) (diffSide, func(), int) {
	if code := checkPassword(options); code != CodeOK {
		return diffSide{}, nil, code
	}
	session, preConnect, code := openSession(ctx, adapters, options)
	if code != CodeOK {
		return diffSide{}, nil, code
	}
	closeSide := func() {
		_ = session.Close()
		preConnect.Stop()
	}

	if schema == "" {
		schema = session.Describe().DefaultSchema
	}
	snapshot, err := schemadiff.ReadSnapshot(ctx, session, schema)
	if err != nil {
		closeSide()
		options.report("%s: %s", options.Profile.Name, db.DescribeError(err))
		return diffSide{}, nil, CodeConnection
	}
	return diffSide{session: session, snapshot: snapshot}, closeSide, CodeOK
}
