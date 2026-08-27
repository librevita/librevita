package http

import (
	"context"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"golang.org/x/crypto/blake2b"

	"librevita.org/internal/core/audit"
	"librevita.org/internal/core/crypto"
	"librevita.org/internal/core/server"
	"librevita.org/internal/core/storage"
	"librevita.org/internal/domain/patient/delivery/views"
	"librevita.org/internal/domain/patient/usecase"
)

// patientDocumentDomain is the attachment namespace for patient files.
const patientDocumentDomain = "patient_document"

// maxDocumentUpload bounds the size of one clinical attachment. The
// route body limit mirrors this value.
const maxDocumentUpload = 25 << 20

// UploadDocument stores a clinical attachment. The patient id comes
// from the path (already policy-filtered), never from the form, so the
// file is bound to the patient the caller is allowed to see. The blob
// is written first and the master index second; a failed index write
// deletes the blob (saga compensation inside the FileManager).
func (h *Handler) UploadDocument(c echo.Context) error {
	ctx := c.Request().Context()
	pt, err := h.patientOr404(c)
	if err != nil {
		return err
	}

	file, err := c.FormFile("file")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "a file is required")
	}
	if file.Size <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "the file is empty")
	}
	if file.Size > maxDocumentUpload {
		return echo.NewHTTPError(http.StatusRequestEntityTooLarge, "the file exceeds the size limit")
	}

	src, err := file.Open()
	if err != nil {
		return err
	}
	defer src.Close()
	if h.engine == nil {
		return errors.New("patient documents: crypto engine is unavailable")
	}
	dek, err := h.engine.EnsurePatientDEKForClinic(ctx, pt.ClinicID, pt.ID)
	if err != nil {
		return err
	}
	defer crypto.ZeroBytes(dek)
	patientURN := crypto.PatientURN(pt.ClinicID, pt.ID)

	name := sanitizeFileName(file.Filename)
	_, err = h.files.UploadEncrypted(ctx, storage.UploadInput{
		Domain:       patientDocumentDomain,
		ResourceID:   pt.ID,
		OriginalName: name,
		ContentType:  contentTypeOr(file.Header.Get("Content-Type"), "application/octet-stream"),
		CreatedBy:    uuid.MustParse(server.ActorID(c)),
	}, src, file.Size, dek, []byte(patientURN))
	if err != nil {
		return err
	}
	// The canonical checksum is witnessed in the append-only chain, so
	// any later modification of the blob is provable.
	h.audit.Record(ctx, server.EventFromRequest(c, audit.AuditResultSuccess,
		"file.upload", "patient:"+pt.ID.String(), "", ""))
	return server.HtmxRedirect(c, "/patients/"+pt.ID.String())
}

// DownloadDocument streams a patient attachment. The file is resolved
// as (domain, patient, file id): a bare id never reaches a file, so an
// id of another patient yields 404 (IDOR protection). Every download
// is written to the audit trail.
func (h *Handler) DownloadDocument(c echo.Context) error {
	ctx := c.Request().Context()
	pt, err := h.patientOr404(c)
	if err != nil {
		return err
	}
	fileID, err := uuid.Parse(c.Param("fileID"))
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound)
	}
	if h.engine == nil {
		return errors.New("patient documents: crypto engine is unavailable")
	}
	dek, err := h.engine.GetPatientDEKForClinic(ctx, pt.ClinicID, pt.ID)
	if err != nil {
		return err
	}
	defer crypto.ZeroBytes(dek)
	patientURN := crypto.PatientURN(pt.ClinicID, pt.ID)
	meta, obj, err := h.files.OpenEncryptedForResource(ctx, patientDocumentDomain, pt.ID, fileID, dek, []byte(patientURN))
	if err != nil {
		if storage.IsNotFound(err) {
			return echo.NewHTTPError(http.StatusNotFound)
		}
		h.audit.Record(ctx, server.EventFromRequest(c, audit.AuditResultFailure,
			"file.read", "patient:"+pt.ID.String(), "", "encrypted object unavailable"))
		return err
	}
	defer obj.Data.Close()
	// The blob must match the checksum witnessed at upload time. The
	// hash is computed while streaming (no buffering); on mismatch the
	// divergence is registered in the append-only trail, so tampering
	// that bypasses the application never stays silent.
	hasher, err := blake2b.New256(nil)
	if err != nil {
		return err
	}
	h.audit.Record(ctx, server.EventFromRequest(c, audit.AuditResultSuccess,
		"file.read", "patient:"+pt.ID.String(), "", ""))
	c.Response().Header().Set("Content-Disposition",
		"attachment; filename=\""+strings.ReplaceAll(meta.OriginalName, `"`, "")+"\"")
	if err := c.Stream(http.StatusOK, meta.ContentType, io.TeeReader(obj.Data, hasher)); err != nil {
		h.audit.Record(ctx, server.EventFromRequest(c, audit.AuditResultFailure,
			"file.read", "patient:"+pt.ID.String(), "", "encrypted stream failed"))
		return err
	}
	if hex.EncodeToString(hasher.Sum(nil)) != meta.Checksum {
		h.audit.Record(ctx, server.EventFromRequest(c, audit.AuditResultFailure,
			"file.read", "patient:"+pt.ID.String(), "", "checksum mismatch"))
	}
	return nil
}

// documentRows lists the patient's attachments, newest first, with the
// display clock applied.
func (h *Handler) documentRows(ctx context.Context, patientID uuid.UUID) ([]views.DocumentRow, error) {
	clock, err := h.userClock(ctx)
	if err != nil {
		return nil, err
	}
	files, err := h.files.List(ctx, patientDocumentDomain, patientID)
	if err != nil {
		return nil, err
	}
	out := make([]views.DocumentRow, 0, len(files))
	for _, f := range files {
		out = append(out, views.DocumentRow{
			ID:           f.ID.String(),
			OriginalName: f.OriginalName,
			ContentType:  f.ContentType,
			Size:         f.Size,
			UploadedAt:   clock.FormatStored(f.CreatedAt),
		})
	}
	return out, nil
}

// patientOr404 loads the patient behind :id or returns 404.
func (h *Handler) patientOr404(c echo.Context) (*usecase.Patient, error) {
	ctx := c.Request().Context()
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return nil, echo.NewHTTPError(http.StatusNotFound)
	}
	clinicID, err := h.clinicID(ctx)
	if err != nil {
		return nil, err
	}
	pt, err := h.svc.Get(ctx, clinicID, id.String())
	if errors.Is(err, usecase.ErrNotFound) {
		return nil, echo.NewHTTPError(http.StatusNotFound)
	}
	if err != nil {
		return nil, err
	}
	return pt, nil
}

// sanitizeFileName keeps only the base name (strip any client path) and
// bounds its length.
func sanitizeFileName(name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	if name == "." || name == "/" || name == "" {
		return "file"
	}
	if len(name) > 200 {
		name = name[:200]
	}
	return name
}

func contentTypeOr(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}
