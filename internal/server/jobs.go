package server

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/ctlplne/healthctl/internal/clinical"
	"github.com/ctlplne/healthctl/internal/store"
)

// Background model calls run as rows in model_jobs. A bounded pool of workers
// claims them, one encounter at a time, recovers from a panic in one call and,
// after a restart, picks up calls that were running. A newer submission to the
// same encounter supersedes an older call: a queued one is skipped and a
// running one is cancelled, so it stops spending tokens on an answer that
// could no longer be saved.

const maxJobAttempts = 3

type jobRunner struct {
	once    sync.Once
	wake    chan struct{}
	mu      sync.Mutex
	running map[string]runningJob // encounter ID
}

type runningJob struct {
	id       int64
	revision int
	cancel   context.CancelFunc
}

type modelJob struct {
	id          int64
	owner       int
	encounterID string
	task        string
	revision    int
	localPlan   bool
	attempts    int
}

func (s *Server) jobRunner() *jobRunner {
	s.jobsInit.Do(func() { s.jobs = &jobRunner{wake: make(chan struct{}, 1), running: map[string]runningJob{}} })
	return s.jobs
}

func workerCount() int {
	n, err := strconv.Atoi(os.Getenv("CLINICAL_MODEL_WORKERS"))
	if err != nil || n < 1 {
		return 2
	}
	return min(n, 8)
}

// startWorkers starts the worker pool once. Jobs that were running when the
// server stopped are queued again first.
func (s *Server) startWorkers() {
	j := s.jobRunner()
	j.once.Do(func() {
		ctx := s.Lifetime
		if ctx == nil {
			ctx = context.Background()
		}
		if _, err := s.Store.DB.ExecContext(ctx, "UPDATE model_jobs SET status='queued' WHERE status='running'"); err != nil && s.Logger != nil {
			s.Logger.Print("event=model_jobs_resume_unavailable")
		}
		for range workerCount() {
			go s.work(ctx)
		}
		j.notify()
	})
}

func (j *jobRunner) notify() {
	select {
	case j.wake <- struct{}{}:
	default:
	}
}

// enqueue records a model call for an encounter revision and wakes a worker.
func (s *Server) enqueue(ctx context.Context, owner int, e *clinical.Encounter, task string, localPlan bool) error {
	s.supersede(ctx, e.ID, e.Revision)
	if _, err := s.Store.DB.ExecContext(ctx, "INSERT INTO model_jobs(owner_id,encounter_id,task,revision,local_plan,status) VALUES($1,$2,$3,$4,$5,'queued')", owner, e.ID, task, e.Revision, localPlan); err != nil {
		return err
	}
	s.startWorkers()
	s.jobRunner().notify()
	return nil
}

// supersede skips queued calls and cancels a running call for older revisions
// of this encounter.
func (s *Server) supersede(ctx context.Context, encounterID string, revision int) {
	_, _ = s.Store.DB.ExecContext(ctx, "UPDATE model_jobs SET status='superseded', finished_at=now() WHERE encounter_id=$1 AND status='queued' AND revision<$2", encounterID, revision)
	j := s.jobRunner()
	j.mu.Lock()
	if r, ok := j.running[encounterID]; ok && r.revision < revision {
		r.cancel()
	}
	j.mu.Unlock()
}

func (s *Server) work(ctx context.Context) {
	j := s.jobRunner()
	for ctx.Err() == nil {
		job, ok, err := s.claim(ctx)
		if err != nil && ctx.Err() == nil && s.Logger != nil {
			s.Logger.Print("event=model_job_claim_unavailable")
		}
		if ok {
			s.runJob(ctx, job)
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-j.wake:
		case <-time.After(5 * time.Second):
		}
	}
}

// claim takes the oldest queued job whose encounter has no running call.
func (s *Server) claim(ctx context.Context) (modelJob, bool, error) {
	var job modelJob
	err := s.Store.DB.QueryRowContext(ctx, `UPDATE model_jobs SET status='running', started_at=now(), attempts=attempts+1
		WHERE id = (SELECT q.id FROM model_jobs q WHERE q.status='queued'
			AND NOT EXISTS (SELECT 1 FROM model_jobs r WHERE r.encounter_id=q.encounter_id AND r.status='running')
			ORDER BY q.id FOR UPDATE SKIP LOCKED LIMIT 1)
		RETURNING id, owner_id, encounter_id, task, revision, local_plan, attempts`).Scan(&job.id, &job.owner, &job.encounterID, &job.task, &job.revision, &job.localPlan, &job.attempts)
	if errors.Is(err, sql.ErrNoRows) {
		return job, false, nil
	}
	return job, err == nil, err
}

func (s *Server) finishJob(id int64, status, reason string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, _ = s.Store.DB.ExecContext(ctx, "UPDATE model_jobs SET status=$2, error=$3, finished_at=now() WHERE id=$1", id, status, reason)
}

// runJob runs one model call. The encounter is read again so a call only runs
// for the revision it was queued for. A panic is recorded as a failed job and
// a recoverable failure the user can retry.
func (s *Server) runJob(parent context.Context, job modelJob) {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	j := s.jobRunner()
	j.mu.Lock()
	j.running[job.encounterID] = runningJob{job.id, job.revision, cancel}
	j.mu.Unlock()
	defer func() {
		j.mu.Lock()
		if r, ok := j.running[job.encounterID]; ok && r.id == job.id {
			delete(j.running, job.encounterID)
		}
		j.mu.Unlock()
	}()
	defer func() {
		if rec := recover(); rec != nil {
			if s.Logger != nil {
				s.Logger.Print("event=model_job_panic")
			}
			s.finishJob(job.id, "failed", "panic")
			s.saveFailure(job)
		}
	}()
	e, err := s.Store.Get(ctx, job.owner, job.encounterID)
	switch {
	case err != nil:
		s.finishJob(job.id, "failed", "encounter_unavailable")
		return
	case e.Revision != job.revision || e.PendingTask() != job.task:
		s.finishJob(job.id, "superseded", "")
		return
	case job.attempts > maxJobAttempts:
		s.finishJob(job.id, "failed", "too_many_attempts")
		s.saveFailure(job)
		return
	}
	if s.beforeModel != nil {
		s.beforeModel(e)
	}
	err = s.finishModel(ctx, job.owner, e, job.task, job.localPlan)
	switch {
	case err == nil:
		s.finishJob(job.id, "done", "")
	case errors.Is(err, store.ErrConflict) || errors.Is(ctx.Err(), context.Canceled) && parent.Err() == nil:
		s.finishJob(job.id, "superseded", "")
	default:
		s.finishJob(job.id, "failed", "save_unavailable")
		if s.Logger != nil {
			s.Logger.Print("event=encounter_completion_unavailable")
		}
	}
}

// saveFailure records a recoverable failure on a fresh copy of the encounter,
// so the history is kept and the user is told to retry.
func (s *Server) saveFailure(job modelJob) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	e, err := s.Store.Get(ctx, job.owner, job.encounterID)
	if err != nil || e.Revision != job.revision || e.PendingTask() != job.task {
		return
	}
	rev := e.Revision
	e.Fail(job.task)
	_ = s.Store.Finish(ctx, job.owner, e, rev)
}
