-- +goose Up
-- +goose NO TRANSACTION
-- Tenant-scoped and hot-path indexes. The model is single-clinic per
-- installation (see internal/domain/clinic/provider.go): clinic_id on
-- clinical tables is future-proofing, and the registry queries filter
-- and order by it, so the composite indexes below carry the page.
--
-- audit_log.resource is queried on every patient detail page
-- (ForResource) with no index today -- a full scan per request.
-- staff_change_requests.requested_by drives the "my requests" list.
-- patients is filtered by clinic + status and ordered by name.

CREATE INDEX idx_audit_log_resource ON audit_log (resource, id DESC);
CREATE INDEX idx_staff_requests_requester ON staff_change_requests (
    requested_by, created_at DESC
);
CREATE INDEX idx_patients_clinic_name ON patients (clinic_id,
display_name COLLATE NOCASE);
CREATE INDEX idx_patients_clinic_status ON patients (clinic_id, status);

-- +goose Down
-- +goose NO TRANSACTION
DROP INDEX idx_patients_clinic_status;
DROP INDEX idx_patients_clinic_name;
DROP INDEX idx_staff_requests_requester;
DROP INDEX idx_audit_log_resource;
