package http

import (
	"bytes"
	"context"
	"errors"
	"html"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"librevita.org/internal/core/server"
	"librevita.org/internal/core/storage"
	"librevita.org/internal/domain/user/usecase"
	"librevita.org/internal/types"
	"librevita.org/internal/ui/shared"
)

// avatarDomain is the attachment namespace for user avatars. It is a
// public-class domain, so avatars are served to every authenticated
// session.
const avatarDomain = "avatar"

// maxAvatarSize bounds one avatar upload (2 MiB).
const maxAvatarSize = 2 << 20

// avatarImageTypes are the accepted image content types, matched
// against the sniffed payload rather than the client header.
var avatarImageTypes = map[string]bool{
	"image/jpeg": true, "image/png": true, "image/webp": true, "image/gif": true,
}

// AvatarPage renders the avatar section of the profile.
// AvatarUpload stores the signed-in user's avatar. The owner is the
// principal from the session — never a form field — and the previous
// avatars are removed only after the new one is stored, so a failed
// upload never leaves the user without a picture.
func (h *Handler) AvatarUpload(c echo.Context) error {
	ctx := c.Request().Context()
	p := server.Principal(c)
	if p == nil {
		return c.Redirect(http.StatusFound, server.LoginPath)
	}

	file, err := c.FormFile("avatar")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "an image is required")
	}
	if file.Size <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "the image is empty")
	}
	if file.Size > maxAvatarSize {
		return echo.NewHTTPError(http.StatusRequestEntityTooLarge, "the image exceeds the size limit")
	}

	src, err := file.Open()
	if err != nil {
		return err
	}
	defer src.Close()

	// Sniff the first bytes: the content type is decided by the
	// payload, not by the client-provided header.
	head := make([]byte, 512)
	n, err := io.ReadFull(src, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return err
	}
	contentType := http.DetectContentType(head[:n])
	if !avatarImageTypes[contentType] {
		return echo.NewHTTPError(http.StatusBadRequest, "the file is not a supported image")
	}
	body := io.MultiReader(bytes.NewReader(head[:n]), src)

	userID := uuid.MustParse(p.ID)
	meta, err := h.files.Upload(ctx, storage.UploadInput{
		Domain:       avatarDomain,
		ResourceID:   userID,
		OriginalName: sanitizeAvatarName(file.Filename),
		ContentType:  contentType,
		CreatedBy:    userID,
	}, body, file.Size)
	if err != nil {
		return err
	}

	// One avatar per user: drop the older pictures now that the new one
	// is safely stored. The freshly stored avatar is excluded, or the
	// cleanup would delete the picture the user just uploaded.
	h.removeAvatars(ctx, userID, meta.ID)

	h.audit.Record(ctx, server.EventFromRequest(c, types.AuditResultSuccess,
		"avatar.update", "user:"+p.ID, meta.OriginalName, ""))
	return server.HtmxRedirect(c, "/profile")
}

// AvatarRemove deletes every avatar of the signed-in user.
func (h *Handler) AvatarRemove(c echo.Context) error {
	ctx := c.Request().Context()
	p := server.Principal(c)
	if p == nil {
		return c.Redirect(http.StatusFound, server.LoginPath)
	}
	userID := uuid.MustParse(p.ID)
	removed := h.removeAvatars(ctx, userID, uuid.Nil)
	h.audit.Record(ctx, server.EventFromRequest(c, types.AuditResultSuccess,
		"avatar.remove", "user:"+p.ID, "", "removed "+itoaAvatar(removed)))
	return server.HtmxRedirect(c, "/profile")
}

// Avatar serves the signed-in user's avatar, falling back to an inline
// SVG placeholder with the initials when there is no picture, so the
// <img> never breaks and no JavaScript is needed.
func (h *Handler) Avatar(c echo.Context) error {
	ctx := c.Request().Context()
	p := server.Principal(c)
	if p == nil {
		return c.Redirect(http.StatusFound, server.LoginPath)
	}
	return h.serveAvatar(ctx, c, uuid.MustParse(p.ID), p.Name)
}

// UserAvatar serves another user's avatar for the admin directory,
// with the same placeholder fallback.
func (h *Handler) UserAvatar(c echo.Context) error {
	ctx := c.Request().Context()
	user, err := h.svc.UserByID(ctx, c.Param("id"))
	if err != nil {
		if errors.Is(err, usecase.ErrUserNotFound) {
			return echo.NewHTTPError(http.StatusNotFound)
		}
		return err
	}
	return h.serveAvatar(ctx, c, user.ID, user.DisplayName)
}

// serveAvatar streams the newest avatar of userID, or the initials
// placeholder when the user has none.
func (h *Handler) serveAvatar(ctx context.Context, c echo.Context, userID uuid.UUID, name string) error {
	avatars, err := h.files.List(ctx, avatarDomain, userID)
	if err != nil {
		return err
	}
	if len(avatars) == 0 {
		c.Response().Header().Set("Content-Type", "image/svg+xml")
		c.Response().WriteHeader(http.StatusOK)
		_, err := c.Response().Write(avatarPlaceholder(name))
		return err
	}
	meta, obj, err := h.files.OpenForResource(ctx, avatarDomain, userID, avatars[0].ID)
	if err != nil {
		if storage.IsNotFound(err) {
			return echo.NewHTTPError(http.StatusNotFound)
		}
		return err
	}
	defer obj.Data.Close()
	c.Response().Header().Set("Cache-Control", "private, max-age=3600")
	return c.Stream(http.StatusOK, meta.ContentType, obj.Data)
}

// removeAvatars deletes every avatar of the user except the given id
// (the freshly uploaded one) and returns how many were removed.
func (h *Handler) removeAvatars(ctx context.Context, userID uuid.UUID, except uuid.UUID) int {
	avatars, err := h.files.List(ctx, avatarDomain, userID)
	if err != nil {
		h.log.Warn("avatar list failed", "user_id", userID, "error", err)
		return 0
	}
	removed := 0
	for _, a := range avatars {
		if a.ID == except {
			continue
		}
		if err := h.files.Delete(ctx, a.ID); err != nil {
			h.log.Warn("avatar remove failed", "file_id", a.ID, "error", err)
			continue
		}
		removed++
	}
	return removed
}

// avatarPlaceholder builds an inline SVG with the user initials, so the
// avatar image slot always renders without JavaScript.
func avatarPlaceholder(name string) []byte {
	initials := shared.Initials(name)
	if len(initials) > 2 {
		initials = initials[:2]
	}
	return []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="128" height="128" viewBox="0 0 128 128">` +
		`<rect width="128" height="128" rx="64" fill="#4f46e5"/>` +
		`<text x="64" y="82" font-family="Arial, sans-serif" font-size="52" fill="#ffffff" text-anchor="middle">` +
		html.EscapeString(initials) + `</text></svg>`)
}

// sanitizeAvatarName keeps only the base name and bounds its length.
func sanitizeAvatarName(name string) string {
	name = strings.TrimSpace(name)
	if i := strings.LastIndexAny(name, "/\\"); i >= 0 {
		name = name[i+1:]
	}
	if name == "" {
		return "avatar"
	}
	if len(name) > 100 {
		name = name[:100]
	}
	return name
}

func itoaAvatar(n int) string {
	return strconv.Itoa(n)
}
