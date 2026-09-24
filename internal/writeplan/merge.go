package writeplan

import (
	"context"
	"slices"
	"strings"

	"github.com/masumedb/masume/internal/db"
)

// MergePlans returns one plan for several writes of one kind on one table: the rows, the
// cascades and the undo of every write added together. It returns false for writes of
// different kinds or tables.
func MergePlans(plans []Plan) (Plan, bool) {
	if len(plans) == 0 {
		return Plan{}, false
	}
	merged := plans[0]
	merged.Columns = slices.Clone(merged.Columns)
	merged.Cascades = slices.Clone(merged.Cascades)
	merged.Blockers = slices.Clone(merged.Blockers)
	statements := []string{merged.SQL}
	for _, plan := range plans[1:] {
		if plan.Kind != merged.Kind || plan.Table != merged.Table {
			return Plan{}, false
		}
		statements = append(statements, plan.SQL)
		for _, column := range plan.Columns {
			if !slices.Contains(merged.Columns, column) {
				merged.Columns = append(merged.Columns, column)
			}
		}
		merged.Rows += plan.Rows
		merged.HasRows = merged.HasRows && plan.HasRows
		if merged.RowsReason == "" {
			merged.RowsReason = plan.RowsReason
		}
		merged.Cascades = mergeCascades(merged.Cascades, plan.Cascades)
		merged.Blockers = mergeCascades(merged.Blockers, plan.Blockers)
		merged.Undo = mergeUndoPlans(merged.Undo, plan.Undo)
	}
	merged.SQL = strings.Join(statements, ";\n")
	return merged, true
}

// mergeCascades adds the cascades of one more write. A cascade of the same cause adds its
// rows, and its referencing predicates are joined with or.
func mergeCascades(held, more []Cascade) []Cascade {
	for _, cascade := range more {
		at := slices.IndexFunc(held, func(kept Cascade) bool {
			return kept.Reason == cascade.Reason && kept.Table == cascade.Table &&
				kept.Trigger == cascade.Trigger
		})
		if at < 0 {
			held = append(held, cascade)
			continue
		}
		held[at].Rows += cascade.Rows
		held[at].HasRows = held[at].HasRows && cascade.HasRows
		if held[at].Referencing != "" && cascade.Referencing != "" {
			held[at].Referencing = "(" + held[at].Referencing + ") or (" +
				cascade.Referencing + ")"
		}
	}
	return held
}

// mergeUndoPlans adds the undo of one more write. The undo is kept only where every write
// keeps one.
func mergeUndoPlans(held, more UndoPlan) UndoPlan {
	held.Rows += more.Rows
	if held.Kept && !more.Kept {
		held.Kept, held.Reason = false, more.Reason
	}
	return held
}

// ApplyWithUndo captures the original rows of every write and applies the writes in one
// transaction when every undo is kept and the server has transactions.
//
// A failed or truncated undo query prevents the writes.
func ApplyWithUndo(
	ctx context.Context, session Writer, plans []UndoPlan, apply func(context.Context) error,
) (Undo, error) {
	if len(plans) == 0 {
		return Undo{}, apply(ctx)
	}
	for _, plan := range plans {
		if !plan.Kept || !session.Capabilities().HasTransactions {
			return Undo{Table: plan.Table, Reason: describeUnkeptUndo(session, plan)}, apply(ctx)
		}
	}

	joined := session.ReadTransactionState() == db.TransactionOpen
	if !joined {
		if err := session.BeginTransaction(ctx); err != nil {
			return Undo{}, err
		}
	}
	undo := Undo{Table: plans[0].Table}
	for _, plan := range plans {
		part, err := ReadUndo(ctx, session, plan)
		if err != nil {
			return Undo{}, endFailedWrite(ctx, session, joined, err)
		}
		undo.Changes = append(undo.Changes, part.Changes...)
		undo.Display = append(undo.Display, part.Display...)
		undo.Rows += part.Rows
	}
	if err := apply(ctx); err != nil {
		return Undo{}, endFailedWrite(ctx, session, joined, err)
	}
	if !joined {
		if err := session.CommitTransaction(ctx); err != nil {
			rollBack(ctx, session)
			return Undo{}, err
		}
	}
	return undo, nil
}
