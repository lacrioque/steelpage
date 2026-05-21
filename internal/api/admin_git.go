package api

import (
	"net/http"
)

// AdminGitStatus returns the current sync state — ahead/behind counts,
// whether a rebase is paused, and the most recent Sync outcome (if any).
func (a *API) AdminGitStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, a.Git.SnapshotStatus(a.Cfg.Repo.PushRemote))
}

// AdminGitPull runs `git pull --rebase`. On conflict the rebase is left
// in place so the operator can resolve or abort.
func (a *API) AdminGitPull(w http.ResponseWriter, _ *http.Request) {
	if !a.Git.HasRemote(a.Cfg.Repo.PushRemote) {
		writeError(w, http.StatusBadRequest, "remote not configured")
		return
	}
	conflicts, err := a.Git.PullRebase(a.Cfg.Repo.PushRemote)
	resp := map[string]any{}
	if err != nil {
		resp["error"] = err.Error()
	}
	if len(conflicts) > 0 {
		resp["conflict"] = true
		resp["files"] = conflicts
	} else {
		resp["pulled"] = err == nil
	}
	resp["status"] = a.Git.SnapshotStatus(a.Cfg.Repo.PushRemote)
	writeJSON(w, http.StatusOK, resp)
}

// AdminGitPush wraps pull-then-push behind a single button. Push is skipped
// if pull surfaced a conflict.
func (a *API) AdminGitPush(w http.ResponseWriter, _ *http.Request) {
	if !a.Git.HasRemote(a.Cfg.Repo.PushRemote) {
		writeError(w, http.StatusBadRequest, "remote not configured")
		return
	}
	result := a.Git.Sync(a.Cfg.Repo.PushRemote)
	writeJSON(w, http.StatusOK, map[string]any{
		"sync":   result,
		"status": a.Git.SnapshotStatus(a.Cfg.Repo.PushRemote),
	})
}

// AdminGitAbort aborts an in-progress rebase, leaving the tree at the
// pre-pull state. No-op when no rebase is active.
func (a *API) AdminGitAbort(w http.ResponseWriter, _ *http.Request) {
	if err := a.Git.AbortRebase(); err != nil {
		logError("git rebase --abort", err)
		writeError(w, http.StatusInternalServerError, "failed to abort rebase")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"aborted": true,
		"status":  a.Git.SnapshotStatus(a.Cfg.Repo.PushRemote),
	})
}
