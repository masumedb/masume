package app

import (
	"slices"
	"testing"
	"time"

	"github.com/masumedb/masume/internal/db"
)

var listedSessions = []db.Activity{
	{PID: 30, User: "carol", State: "idle", Duration: time.Second, Query: "select 3"},
	{PID: 10, User: "alice", State: "active", Duration: time.Minute, Query: "update orders"},
	{PID: 20, User: "bob", State: "idle in transaction", Duration: time.Hour, Query: "begin"},
	{PID: 40, User: "dave", State: "Sleep", Duration: 0, Query: ""},
}

// readPIDs returns the PIDs of the sessions in their order.
func readPIDs(sessions []db.Activity) []int64 {
	pids := []int64{}
	for _, session := range sessions {
		pids = append(pids, session.PID)
	}
	return pids
}

func TestListShownSessionsSortsAndHidesTheIdleOnes(t *testing.T) {
	for _, held := range []struct {
		name string
		view DashboardView
		want []int64
	}{
		{"the order of the server", DashboardView{}, []int64{30, 10, 20, 40}},
		{"pid up", DashboardView{SortBy: ActivityByPID}, []int64{10, 20, 30, 40}},
		{"time, longest first", DashboardView{SortBy: ActivityByTime, Descending: true},
			[]int64{20, 10, 30, 40}},
		{"user up", DashboardView{SortBy: ActivityByUser}, []int64{10, 20, 30, 40}},
		{"idle hidden", DashboardView{HidesIdle: true}, []int64{10, 20}},
	} {
		if got := readPIDs(ListShownSessions(listedSessions, held.view)); !slices.Equal(got, held.want) {
			t.Errorf("%s listed %v, wanted %v", held.name, got, held.want)
		}
	}
	if CountIdleSessions(listedSessions) != 2 {
		t.Errorf("the idle count is %d", CountIdleSessions(listedSessions))
	}
}

func TestSortingByTheSameColumnReversesIt(t *testing.T) {
	view := DashboardView{}
	view.SortSessionsBy(ActivityByTime)
	if view.SortBy != ActivityByTime || !view.Descending {
		t.Fatalf("the first sort by time left %+v", view)
	}
	view.SortSessionsBy(ActivityByTime)
	if view.Descending {
		t.Error("the second sort by time did not reverse it")
	}
	view.SortSessionsBy(ActivityByPID)
	if view.SortBy != ActivityByPID || view.Descending {
		t.Errorf("the sort by pid left %+v", view)
	}
}

func TestStepSessionSortReturnsToTheOrderOfTheServer(t *testing.T) {
	view := DashboardView{}
	seen := []ActivityColumn{}
	for range len(ActivityColumns) + 1 {
		view.StepSessionSort()
		seen = append(seen, view.SortBy)
	}
	want := append(slices.Clone(ActivityColumns), ActivityByServer)
	if !slices.Equal(seen, want) {
		t.Errorf("the steps went through %v", seen)
	}
}
