package headless

import (
	"context"
	"fmt"

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
}

// The exit codes of `masume diff`.
const (
	CodeSame      = 0
	CodeDifferent = 1
)

// RunDiff opens both connections, writes the differences, and returns the exit code.
func RunDiff(ctx context.Context, adapters engines.Adapters, options DiffOptions) int {
	source, code := readDiffSnapshot(ctx, adapters, options.Options, options.SourceSchema)
	if code != CodeOK {
		return code
	}
	targetSchema := options.TargetSchema
	if targetSchema == "" {
		targetSchema = options.SourceSchema
	}
	target, code := readDiffSnapshot(ctx, adapters, options.Target, targetSchema)
	if code != CodeOK {
		return code
	}

	found := schemadiff.Compare(source, target)
	if _, err := fmt.Fprint(options.Out, schemadiff.WriteReport(found)); err != nil {
		options.report("cannot write the report: %s", err)
		return CodeConnection
	}
	if len(found) > 0 {
		return CodeDifferent
	}
	return CodeSame
}

// readDiffSnapshot reads one schema of one connection. Every failure is exit code 2.
func readDiffSnapshot(
	ctx context.Context, adapters engines.Adapters, options Options, schema string,
) (schemadiff.Snapshot, int) {
	if code := checkPassword(options); code != CodeOK {
		return schemadiff.Snapshot{}, code
	}
	session, preConnect, code := openSession(ctx, adapters, options)
	if code != CodeOK {
		return schemadiff.Snapshot{}, code
	}
	defer func() {
		_ = session.Close()
		preConnect.Stop()
	}()

	if schema == "" {
		schema = session.Describe().DefaultSchema
	}
	snapshot, err := schemadiff.ReadSnapshot(ctx, session, schema)
	if err != nil {
		options.report("%s: %s", options.Profile.Name, db.DescribeError(err))
		return schemadiff.Snapshot{}, CodeConnection
	}
	return snapshot, CodeOK
}
