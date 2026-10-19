// Copyright 2026 The OpenSandbox Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Egress-held vault persistence (OSEP-0022 stage 2): one file per subject
// under <egress-volume>/vault/, so an egress restart recovers locally — no
// server re-push exists or is needed. Ordering is persist → apply → ack
// (acked ⇒ durable and applied), reusing the policy-persistence machinery's
// three-state classification, snapshot restore, and recovery latch.
package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/alibaba/opensandbox/egress/pkg/credentialvault"
	"github.com/alibaba/opensandbox/egress/pkg/log"
	"github.com/alibaba/opensandbox/egress/pkg/policy"
	"github.com/alibaba/opensandbox/egress/pkg/subject"
)

// vaultRecoveryState is the recovery latch payload.
type vaultRecoveryState struct {
	reason string
}

// vaultFileStore is the storage seam for one subject's vault file; the real
// store is policy.AtomicPolicyFile, and tests substitute outcomes here.
// Callers must serialize operations across a transaction — the relay write
// path holds the server lock for exactly that.
type vaultFileStore interface {
	Snapshot() (*policy.PolicyFileSnapshot, error)
	SaveRaw(data []byte) (policy.FileMutationState, error)
	Restore(*policy.PolicyFileSnapshot) (policy.FileMutationState, error)
	Delete() (policy.FileMutationState, error)
}

// vaultPersistence owns the per-subject vault files: <dir>/<sandboxUid>.json,
// with lazily constructed, cached file stores.
type vaultPersistence struct {
	dir string
	// newFile is injectable for tests; the default is the atomic file machinery.
	newFile func(path string) (vaultFileStore, error)

	mu    sync.Mutex
	files map[subject.Subject]vaultFileStore
}

// newVaultPersistence prepares the vault directory (0700 — v1 is plaintext,
// the volume joins the credential trust surface) and sweeps staged leftovers:
// startup loads the committed files only.
func newVaultPersistence(dir string) (*vaultPersistence, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return nil, errors.New("vault persistence directory is empty")
	}
	if !filepath.IsAbs(dir) {
		return nil, fmt.Errorf("vault persistence directory %q must be absolute", dir)
	}
	for _, part := range strings.Split(dir, string(os.PathSeparator)) {
		if part == ".." {
			return nil, fmt.Errorf("vault persistence directory %q must not traverse parents", dir)
		}
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("prepare vault directory %s: %w", dir, err)
	}
	// Reject a symlinked vault directory BEFORE chmod/sweep act through it
	// (the per-file atomic machinery re-validates no-follow on every real
	// vault op; this keeps startup from mutating a link target first).
	if info, err := os.Lstat(dir); err != nil {
		return nil, fmt.Errorf("inspect vault directory %s: %w", dir, err)
	} else if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("vault persistence directory %q must not be a symlink", dir)
	} else if !info.IsDir() {
		return nil, fmt.Errorf("vault persistence directory %q is not a directory", dir)
	}
	// An existing directory may carry wider bits from a previous generation.
	if err := os.Chmod(dir, 0o700); err != nil {
		return nil, fmt.Errorf("restrict vault directory %s: %w", dir, err)
	}
	if err := sweepVaultTempFiles(dir); err != nil {
		return nil, fmt.Errorf("sweep staged vault leftovers in %s: %w", dir, err)
	}
	return &vaultPersistence{
		dir:     dir,
		newFile: func(path string) (vaultFileStore, error) { return policy.NewAtomicPolicyFile(path) },
		files:   make(map[subject.Subject]vaultFileStore),
	}, nil
}

// sweepVaultTempFiles unlinks staging files a crashed writer left behind
// (rename never attempted — unconfirmed candidate state).
func sweepVaultTempFiles(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasPrefix(name, policy.AtomicTempPrefix) || !strings.HasSuffix(name, policy.AtomicTempSuffix) {
			continue
		}
		if err := os.Remove(filepath.Join(dir, name)); err != nil {
			return fmt.Errorf("discard staged leftover %s: %w", name, err)
		}
		log.Infof("vault persistence: discarded staged leftover %s", name)
	}
	return nil
}

// vaultFileUID strips the internal "s-" prefix: the file contract names files
// <sandboxUid>.json.
func vaultFileUID(subj subject.Subject) string {
	return strings.TrimPrefix(string(subj), "s-")
}

func (p *vaultPersistence) pathFor(subj subject.Subject) (string, error) {
	uid := vaultFileUID(subj)
	// The relay validates the path-derived UID; envelope UIDs get the same check.
	if !validVaultSubjectID(uid) {
		return "", fmt.Errorf("subject uid %q cannot name a vault file", uid)
	}
	return filepath.Join(p.dir, uid+".json"), nil
}

// storeFor returns the subject's file store, constructing it on first use.
func (p *vaultPersistence) storeFor(subj subject.Subject) (vaultFileStore, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if fs, ok := p.files[subj]; ok {
		return fs, nil
	}
	path, err := p.pathFor(subj)
	if err != nil {
		return nil, err
	}
	fs, err := p.newFile(path)
	if err != nil {
		return nil, err
	}
	p.files[subj] = fs
	return fs, nil
}

// purge removes the subject's file and evicts the cached store. An
// already-absent file is a complete purge; an uncertain outcome is reported —
// the leftover stays fenced out by its recorded runtime fence either way.
// The seal marker (if any) goes with it.
func (p *vaultPersistence) purge(subj subject.Subject) error {
	fs, err := p.storeFor(subj)
	if err != nil {
		return err
	}
	state, err := fs.Delete()
	if err == nil || state == policy.FileUnchanged {
		p.mu.Lock()
		delete(p.files, subj)
		p.mu.Unlock()
	}
	if err != nil {
		return err
	}
	if state == policy.FileUnknown {
		return errors.New("vault file unlink outcome uncertain")
	}
	return p.unseal(subj)
}

// markerFor names the durable seal marker: export writes it before acking
// sealed, and a handler restart between export and REMOVE_BINDING restores
// the seal from it — otherwise binding replay would reload the vault writable
// with a bundle already taken.
func (p *vaultPersistence) markerFor(subj subject.Subject) (string, error) {
	path, err := p.pathFor(subj)
	if err != nil {
		return "", err
	}
	return path + ".sealed", nil
}

// seal writes the marker durably. Presence is the signal; no fsync — the
// marker only needs to survive a container restart, and a torn write still
// exists.
func (p *vaultPersistence) seal(subj subject.Subject) error {
	path, err := p.markerFor(subj)
	if err != nil {
		return err
	}
	return os.WriteFile(path, []byte("sealed"), 0o600)
}

// unseal removes the marker; an absent marker is a complete unseal.
func (p *vaultPersistence) unseal(subj subject.Subject) error {
	path, err := p.markerFor(subj)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// isSealed reports whether the durable seal marker exists.
func (p *vaultPersistence) isSealed(subj subject.Subject) bool {
	path, err := p.markerFor(subj)
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}

// SetVaultFiles wires persistence under the relay handlers; nil (the default)
// keeps the vault memory-only.
func (s *fastSandboxPolicyServer) SetVaultFiles(vp *vaultPersistence) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.vaultFiles = vp
}

// purgeVaultFile removes the subject's persisted slice (rebind or terminal
// cleanup). Best effort: a failed purge leaves a file that its recorded fence
// keeps out of every later registration.
func (s *fastSandboxPolicyServer) purgeVaultFile(subj subject.Subject) {
	if s.vaultFiles == nil {
		return
	}
	if err := s.vaultFiles.purge(subj); err != nil {
		log.Errorf("subject %s: vault file purge failed (leftover stays fenced out): %v", subj, err)
	}
}

// persistVaultCandidateLocked runs the commit protocol; callers hold s.mu for
// the whole operation, which serializes per-subject writers. persist performs
// the durable effect and returns its three-state classification; apply
// commits the prepared candidate and must leave the store untouched on
// failure (a failed apply only needs the file rolled back).
func (s *fastSandboxPolicyServer) persistVaultCandidateLocked(
	subj subject.Subject,
	persist func(vaultFileStore) (policy.FileMutationState, error),
	apply func() error,
) error {
	fs, err := s.vaultFiles.storeFor(subj)
	if err != nil {
		return fmt.Errorf("vault file store unavailable: %w", err)
	}
	before, err := fs.Snapshot()
	if err != nil {
		return fmt.Errorf("vault file snapshot: %w", err)
	}
	state, err := persist(fs)
	switch {
	case err == nil && state == policy.FileCommitted:
		// Rename + parent-dir fsync done.
	case err == nil && state == policy.FileUnchanged:
		// Delete of an already-absent file (SaveRaw never returns this pair).
	case err == nil:
		// A success that did not confirm an effect is uncertain.
		state = policy.FileUnknown
		err = errors.New("vault file persist did not confirm durable completion")
	}
	if err != nil && state == policy.FileUnchanged {
		// Pre-rename failure: last-good intact — NACK without restore or latch.
		return err
	}
	if err != nil {
		// Publication attempted, durability unconfirmed: latch first (only a
		// restart reconciles), then restore so disk agrees with the untouched
		// live state.
		s.requireVaultRecoveryLocked("vault file persist outcome uncertain")
		if restoreErr := restoreVaultFile(fs, before); restoreErr != nil {
			log.Errorf("subject %s: vault file restore after uncertain persist: %v", subj, restoreErr)
			return fmt.Errorf("%w (restore also failed: %v)", err, restoreErr)
		}
		return err
	}
	// Durable. A failed apply rolls the file back — no latch while the
	// restore is confirmed.
	if applyErr := apply(); applyErr != nil {
		if restoreErr := restoreVaultFile(fs, before); restoreErr != nil {
			s.requireVaultRecoveryLocked("vault file restore after failed apply was inconclusive")
			log.Errorf("subject %s: vault file restore after failed apply: %v", subj, restoreErr)
			return fmt.Errorf("vault apply failed (%v) and restore failed (%v)", applyErr, restoreErr)
		}
		return fmt.Errorf("vault apply: %w", applyErr)
	}
	// Durable AND applied: a degraded vault recovers here.
	s.clearVaultDegradedLocked(subj)
	return nil
}

// restoreVaultFile reinstates the previous image and requires the machinery
// to confirm durable completion.
func restoreVaultFile(fs vaultFileStore, before *policy.PolicyFileSnapshot) error {
	state, err := fs.Restore(before)
	if err == nil && state != policy.FileCommitted {
		err = errors.New("vault file restore did not confirm durable completion")
	}
	return err
}

// requireVaultRecoveryLocked latches recovery (first reason wins): vault
// writes freeze until the process restarts and reconciles from the committed
// files. The latch is an atomic so the healthz probe reads it lock-free.
func (s *fastSandboxPolicyServer) requireVaultRecoveryLocked(reason string) {
	if s.vaultRecovery.Load() == nil {
		s.vaultRecovery.CompareAndSwap(nil, &vaultRecoveryState{reason: reason})
		log.Errorf("vault persistence recovery latched (writes frozen until restart): %s", reason)
	}
}

// vaultRecoveryActive reports whether the recovery latch freezes writes.
func (s *fastSandboxPolicyServer) vaultRecoveryActive() bool {
	return s.vaultRecovery.Load() != nil
}

// clearVaultDegradedLocked clears the subject's degraded mark after a
// successful relay write — a corrupt-file subject recovers without recreation.
func (s *fastSandboxPolicyServer) clearVaultDegradedLocked(subj subject.Subject) {
	if e, ok := s.vaults[subj]; ok {
		e.degraded = ""
		s.vaults[subj] = e
	}
}

// degradedVaultUIDs lists the subjects with a degraded vault, as raw sandbox
// UIDs, sorted for stable surfacing.
func (s *fastSandboxPolicyServer) degradedVaultUIDs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.degradedVaultUIDsLocked()
}

// vaultHealthIssue is non-empty only for the process-level failure — the
// recovery latch. A per-subject degraded vault surfaces via the status
// payload's degradedVaults instead (see rehydrateVaultReload).
func (s *fastSandboxPolicyServer) vaultHealthIssue() string {
	if state := s.vaultRecovery.Load(); state != nil {
		return "vault persistence recovery required: " + state.reason
	}
	return ""
}

// degradedVaultUIDsLocked implements degradedVaultUIDs; callers hold s.mu.
func (s *fastSandboxPolicyServer) degradedVaultUIDsLocked() []string {
	var out []string
	for subj, e := range s.vaults {
		if e.degraded != "" {
			out = append(out, vaultFileUID(subj))
		}
	}
	sort.Strings(out)
	return out
}

// rehydrateVaultReload is the egress-restart recovery path: at (re)registration
// of a subject with no live vault slice, load its committed file, fenced by
// the full runtime identity:
//
//   - fence match → reload at the exact file revision
//   - mismatch → stale, purged (a reset — or an attachment-only rebind whose
//     purge outcome was uncertain — never inherits the predecessor's vault)
//   - corrupt/unreadable, or content the binding policy rejects → the
//     subject's vault is degraded (interception fails closed, policy
//     unaffected) and the file is quarantined so a restart cannot re-degrade
//     on it — except an unknown version, preserved for the newer writer. A
//     successful relay write clears the mark
//
// Removal SET_BINDINGS skip the reload: with no policy to revalidate against,
// the file stays fenced on disk until the next registration or terminal purge.
// An existing slice for this exact fence wins (duplicate SET_BINDING); one
// left under a different fence goes through the same file rules as a fresh
// registration.
func (s *fastSandboxPolicyServer) rehydrateVaultReload(subj subject.Subject, fence subject.Fencing, pol *policy.NetworkPolicy) {
	if s.vaultFiles == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if e, live := s.vaults[subj]; live {
		if e.fence == fence {
			return
		}
		delete(s.vaults, subj)
	}
	entry := vaultEntry{store: credentialvault.NewStore(nil, func() bool { return true }), fence: fence}
	fs, err := s.vaultFiles.storeFor(subj)
	if err != nil {
		entry.degraded = fmt.Sprintf("vault file store unavailable: %v", err)
		s.vaults[subj] = entry
		log.Warnf("subject %s: %s", subj, entry.degraded)
		return
	}
	snap, err := fs.Snapshot()
	switch {
	case err != nil:
		entry.degraded = fmt.Sprintf("vault file unreadable: %v", err)
		log.Warnf("subject %s: %s", subj, entry.degraded)
	case !snap.Exists():
		// Fresh subject: nothing persisted, nothing to recover.
	default:
		vf, body, decErr := credentialvault.DecodeVaultFile(snap.Data())
		switch {
		case decErr != nil && errors.Is(decErr, credentialvault.ErrUnsupportedFileVersion):
			// Written by a newer consumer — reject without destroying.
			entry.degraded = fmt.Sprintf("vault file version not readable by this egress: %v", decErr)
			log.Warnf("subject %s: %s (file preserved for the newer writer)", subj, entry.degraded)
		case decErr != nil:
			// Structurally corrupt: quarantine so the degradation cannot
			// outlive this process via the file.
			entry.degraded = fmt.Sprintf("corrupt vault file: %v", decErr)
			s.quarantineVaultFile(subj, fs)
			log.Warnf("subject %s: %s", subj, entry.degraded)
		case (subject.Fencing{RuntimeInstanceID: vf.Incarnation, AttachmentID: vf.AttachmentID}).Matches(fence):
			// Same-instance recovery: revalidate against the replayed binding
			// policy, install at the exact file revision.
			if err := entry.store.Reload(vf.Revision, credentialvault.CreateRequest(body), pol); err != nil {
				entry.degraded = fmt.Sprintf("vault file reload rejected: %v", err)
				s.quarantineVaultFile(subj, fs) // can never validate under this binding
				log.Warnf("subject %s: %s", subj, entry.degraded)
			} else {
				log.Infof("subject %s: vault reloaded from file (revision %d, incarnation %s)", subj, vf.Revision, vf.Incarnation)
			}
		default:
			// Stale: discard durably. A failed purge is harmless — the
			// recorded fence mismatches every later registration regardless.
			if _, delErr := fs.Delete(); delErr != nil {
				log.Warnf("subject %s: stale vault file purge failed (stays fenced out): %v", subj, delErr)
			} else {
				log.Infof("subject %s: stale vault file discarded (fence %s/%s ≠ %s/%s)", subj, vf.Incarnation, vf.AttachmentID, fence.RuntimeInstanceID, fence.AttachmentID)
			}
		}
	}
	// A durable seal marker (export acked, teardown not yet delivered)
	// restores the sealed entry across a restart: vault ops stay 409 until
	// REMOVE_BINDING purges or an import unseals.
	if s.vaultFiles.isSealed(subj) {
		entry.sealed = true
		log.Infof("subject %s: seal restored from marker (exported bundle pending)", subj)
	}
	s.vaults[subj] = entry
}

// quarantineVaultFile unlinks a file that can never load again. Best effort:
// on failure the file stays and the subject remains degraded.
func (s *fastSandboxPolicyServer) quarantineVaultFile(subj subject.Subject, fs vaultFileStore) {
	if _, err := fs.Delete(); err != nil {
		log.Errorf("subject %s: vault file quarantine failed (file left in place): %v", subj, err)
	}
}
