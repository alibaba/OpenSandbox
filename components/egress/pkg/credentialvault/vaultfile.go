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

// The per-subject vault file format (OSEP-0022 "Vault Storage"): a versioned
// server↔egress persistence contract, one file per subject at
// <egress-volume>/vault/<sandboxUid>.json. Conformance vectors live in
// testdata/vaultfile and pin the exact encoding both sides must agree on.
package credentialvault

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// VaultFileVersion is the only supported on-disk schema version. Anything
// else — including a missing one — is rejected rather than misinterpreted;
// the rejection is identifiable via ErrUnsupportedFileVersion so callers can
// preserve files written by a newer consumer.
const VaultFileVersion = 1

// MaxEncodedVaultFileBytes bounds one encoded vault file: a relay body up to
// maxCredentialVaultBodyBytes plus the envelope (version, fence, revision,
// JSON framing). Consumers sizing storage or transfers for vault files use
// this instead of re-deriving the body limit — a cap at the body size alone
// would reject files the egress itself wrote.
const MaxEncodedVaultFileBytes = maxCredentialVaultBodyBytes + 4<<10

// ErrUnsupportedFileVersion marks a DecodeVaultFile failure caused solely by
// an unknown schema version — structurally fine for a newer reader.
var ErrUnsupportedFileVersion = errors.New("unsupported vault file version")

// VaultFile is the versioned envelope of one subject's persisted vault
// revision. Incarnation + AttachmentID record the owning runtime fence from
// the SET_BINDING envelope; a registration under a different fence — a reset,
// or a rebind that changed only the attachment — finds the file stale.
type VaultFile struct {
	Version      int             `json:"v"`
	Incarnation  string          `json:"incarnation"`
	AttachmentID string          `json:"attachmentId"`
	Revision     int64           `json:"revision"`
	Vault        json.RawMessage `json:"vault"`
}

// VaultFileBody is the OSEP-0012 vault JSON carried in VaultFile.Vault —
// exactly the credential vault create shape.
type VaultFileBody struct {
	Credentials []Credential `json:"credentials"`
	Bindings    []Binding    `json:"bindings"`
}

// EncodeVaultFile renders the canonical bytes: version pinned to
// VaultFileVersion, the owning runtime fence, the revision, the vault body.
// The output is deterministic (sorted content comes from the callers) and
// ends with a trailing newline, matching the policy file convention.
func EncodeVaultFile(incarnation, attachmentID string, revision int64, body VaultFileBody) ([]byte, error) {
	incarnation = strings.TrimSpace(incarnation)
	if incarnation == "" {
		return nil, fmt.Errorf("vault file incarnation cannot be blank")
	}
	attachmentID = strings.TrimSpace(attachmentID)
	if attachmentID == "" {
		return nil, fmt.Errorf("vault file attachmentId cannot be blank")
	}
	if revision < 1 {
		return nil, fmt.Errorf("vault file revision %d must be positive", revision)
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("encode vault file body: %w", err)
	}
	out, err := json.Marshal(VaultFile{
		Version:      VaultFileVersion,
		Incarnation:  incarnation,
		AttachmentID: attachmentID,
		Revision:     revision,
		Vault:        raw,
	})
	if err != nil {
		return nil, fmt.Errorf("encode vault file envelope: %w", err)
	}
	return append(out, '\n'), nil
}

// DecodeVaultFile parses and validates file bytes. The version discriminator
// is read FIRST with a tolerant view: a newer envelope carries unknown
// fields, so the strict v1 schema must not reject (and the caller quarantine)
// a file this reader should merely preserve. Only strictly newer versions
// surface ErrUnsupportedFileVersion; a missing or invalid version is plain
// corruption. The v1 schema then enforces exactly the known fields, non-blank
// fence fields, a positive revision, and a decodable create-shaped body;
// semantic validation (credential references, policy coverage, ambiguity) is
// the store's job at load time.
func DecodeVaultFile(data []byte) (VaultFile, VaultFileBody, error) {
	var body VaultFileBody
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return VaultFile{}, body, fmt.Errorf("vault file is empty")
	}
	var disc struct {
		Version *int `json:"v"`
	}
	if err := json.Unmarshal(trimmed, &disc); err != nil {
		return VaultFile{}, body, fmt.Errorf("decode vault file envelope: %w", err)
	}
	if disc.Version != nil && *disc.Version > VaultFileVersion {
		return VaultFile{}, body, fmt.Errorf("%w %d (newer than supported %d)", ErrUnsupportedFileVersion, *disc.Version, VaultFileVersion)
	}
	var vf VaultFile
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&vf); err != nil {
		return VaultFile{}, body, fmt.Errorf("decode vault file envelope: %w", err)
	}
	if vf.Version != VaultFileVersion {
		return VaultFile{}, body, fmt.Errorf("invalid vault file version %d (want %d)", vf.Version, VaultFileVersion)
	}
	if strings.TrimSpace(vf.Incarnation) == "" {
		return VaultFile{}, body, fmt.Errorf("vault file incarnation cannot be blank")
	}
	if strings.TrimSpace(vf.AttachmentID) == "" {
		return VaultFile{}, body, fmt.Errorf("vault file attachmentId cannot be blank")
	}
	if vf.Revision < 1 {
		return VaultFile{}, body, fmt.Errorf("vault file revision %d must be positive", vf.Revision)
	}
	if len(vf.Vault) == 0 {
		return VaultFile{}, body, fmt.Errorf("vault file body is missing")
	}
	bodyDec := json.NewDecoder(bytes.NewReader(vf.Vault))
	bodyDec.DisallowUnknownFields()
	if err := bodyDec.Decode(&body); err != nil {
		return VaultFile{}, body, fmt.Errorf("decode vault file body: %w", err)
	}
	return vf, body, nil
}
