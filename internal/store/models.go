package store

import "time"

const (
	StatusRunning = "running"
	StatusSuccess = "success"
	StatusFailed  = "failed"
	StatusKilled  = "killed"
	StatusUnknown = "unknown"

	ModeRun   = "run"
	ModeShell = "shell"
	ModeRerun = "rerun"
)

type Run struct {
	ID                 int64
	Status             string
	Mode               string
	Command            string
	ArgvJSON           string
	CWD                string
	StartedAt          time.Time
	FinishedAt         *time.Time
	DurationMS         *int64
	ExitCode           *int
	Hostname           string
	Shell              string
	GitRoot            string
	GitBranch          string
	GitCommit          string
	GitDirty           *bool
	StdoutPath         string
	StderrPath         string
	TerminalOutputPath string
	NotifyRequested    bool
	NotifyAlways       bool
	CreatedAt          time.Time
	Label              string
	EnvJSON            string
}

type CreateRun struct {
	Status             string
	Mode               string
	Command            string
	ArgvJSON           string
	CWD                string
	StartedAt          time.Time
	Hostname           string
	Shell              string
	GitRoot            string
	GitBranch          string
	GitCommit          string
	GitDirty           *bool
	StdoutPath         string
	StderrPath         string
	TerminalOutputPath string
	NotifyRequested    bool
	NotifyAlways       bool
	EnvJSON            string
}

type Stats struct {
	TotalRuns      int64
	CompletedRuns  int64
	SuccessfulRuns int64
	FailedRuns     int64
	RunningRuns    int64
	ActiveDays     int64
	FirstStartedAt *time.Time
	LastStartedAt  *time.Time
	AvgDurationMS  *int64
	SlowestRuns    []SlowRun
}

type SlowRun struct {
	ID         int64
	Status     string
	Mode       string
	Command    string
	ArgvJSON   string
	CWD        string
	StartedAt  time.Time
	DurationMS int64
}

type SearchResult struct {
	Run     Run
	Snippet string
}
