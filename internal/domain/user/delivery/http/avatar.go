package http

import (
	"bytes"
	"context"
	"errors"
	"html"
	"image"
	"image/color"
	"image/draw"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/disintegration/imaging"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	// The standard library registers png, jpeg and gif (through
	// imaging); the extra sniffed formats need their supplementary
	// decoders registered here or processAvatar would reject bmp, tiff
	// and webp uploads as undecodable.
	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"

	"librevita.org/internal/core/audit"
	"librevita.org/internal/core/crypto"
	"librevita.org/internal/core/server"
	"librevita.org/internal/core/storage"
	"librevita.org/internal/domain/user/usecase"
	"librevita.org/internal/ui/shared"
)

// avatarDomain is the attachment namespace for user avatars. It is a
// public-class domain, so avatars are served to every authenticated
// session.
const avatarDomain = "avatar"

// maxAvatarSize bounds one avatar upload (2 MiB).
const maxAvatarSize = 2 << 20

// avatarImageTypes are the accepted image content types, matched
// against the sniffed payload rather than the client header. Every
// accepted source is processed down to one JPEG, whatever its origin.
var avatarImageTypes = map[string]bool{
	"image/jpeg": true, "image/png": true, "image/webp": true, "image/gif": true,
	"image/bmp": true, "image/tiff": true,
}

// avatarSize is the square side every avatar is processed to (centered
// crop), small enough for the topbar and profile without heavy payloads.
const avatarSize = 256

// avatarMaxDimension caps the decoded dimensions before the full decode,
// so a tiny file declaring a huge canvas (decompression bomb) is
// rejected without allocating it.
const avatarMaxDimension = 5000

// avatarJPEGQuality trades size for fidelity; 85 keeps photos small
// while staying visually lossless for profile pictures.
const avatarJPEGQuality = 85

// AvatarPage renders the avatar section of the profile.
// AvatarUpload stores the signed-in user's avatar. The owner is the
// principal from the session — never a form field — and the previous
// avatars are removed only after the new one is stored, so a failed
// upload never leaves the user without a picture. Failures render the
// profile page with the error message: the form submits natively, so
// an error body would land bare on the browser.
func (h *Handler) AvatarUpload(c echo.Context) error {
	ctx := c.Request().Context()
	p := server.Principal(c)
	if p == nil {
		return c.Redirect(http.StatusFound, server.LoginPath)
	}

	file, err := c.FormFile("avatar")
	if err != nil {
		return h.profilePage(ctx, c, p, http.StatusBadRequest, "an image is required")
	}
	if file.Size <= 0 {
		return h.profilePage(ctx, c, p, http.StatusBadRequest, "the image is empty")
	}
	if file.Size > maxAvatarSize {
		return h.profilePage(ctx, c, p, http.StatusBadRequest, "the image exceeds the size limit")
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
		return h.profilePage(ctx, c, p, http.StatusBadRequest, "the file is not a supported image")
	}

	// The upload is bounded by the route body limit (2 MiB), so the
	// payload fits in memory; buffer it to decode twice (config first
	// for the dimension check, then the full image).
	payload, err := io.ReadAll(io.MultiReader(bytes.NewReader(head[:n]), src))
	if err != nil {
		return err
	}
	processed, err := processAvatar(payload)
	if err != nil {
		return h.profilePage(ctx, c, p, http.StatusBadRequest, err.Error())
	}

	userID := uuid.MustParse(p.ID)
	meta, err := h.files.Upload(ctx, storage.UploadInput{
		Domain:       avatarDomain,
		ResourceID:   userID,
		OriginalName: sanitizeAvatarName(file.Filename),
		ContentType:  "image/jpeg",
		CreatedBy:    userID,
	}, bytes.NewReader(processed), int64(len(processed)))
	if err != nil {
		return err
	}

	// One avatar per user: drop the older pictures now that the new one
	// is safely stored. The freshly stored avatar is excluded, or the
	// cleanup would delete the picture the user just uploaded.
	h.removeAvatars(ctx, userID, meta.ID)

	// The canonical checksum is witnessed in the append-only chain.
	h.audit.Record(ctx, server.EventFromRequest(c, audit.AuditResultSuccess,
		"avatar.update", "user:"+p.ID, meta.OriginalName, "checksum: "+meta.Checksum))
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
	h.audit.Record(ctx, server.EventFromRequest(c, audit.AuditResultSuccess,
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
// placeholder when the user has none. Both payloads share the caching
// contract: private (the picture is per-user, never for shared caches),
// no-cache (revalidate before every use, so an upload reflects on the
// very next navigation), and a strong ETag so the revalidation answers
// 304 without the payload.
func (h *Handler) serveAvatar(ctx context.Context, c echo.Context, userID uuid.UUID, name string) error {
	avatars, err := h.files.List(ctx, avatarDomain, userID)
	if err != nil {
		return err
	}
	if len(avatars) == 0 {
		payload := avatarPlaceholder(name)
		if !avatarCacheHeaders(c, "ph-"+avatarDigest(payload)) {
			return nil
		}
		c.Response().Header().Set("Content-Type", "image/svg+xml")
		c.Response().WriteHeader(http.StatusOK)
		_, err := c.Response().Write(payload)
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
	if !avatarCacheHeaders(c, meta.ETag) {
		return nil
	}
	return c.Stream(http.StatusOK, meta.ContentType, obj.Data)
}

// avatarCacheHeaders sets the shared caching contract and answers 304
// when the client validates with an If-None-Match that still matches;
// it reports whether the caller should proceed to send the body. The
// representation is revalidated on every use (no-cache) rather than
// held for a long max-age: an upload must show up on the next page
// load, not an hour later, and the ETag keeps that revalidation free
// of payloads.
func avatarCacheHeaders(c echo.Context, etag string) bool {
	c.Response().Header().Set("Cache-Control", "private, no-cache")
	quoted := `"` + etag + `"`
	c.Response().Header().Set("ETag", quoted)
	if inm := c.Request().Header.Get("If-None-Match"); inm != "" && etagMatches(inm, quoted) {
		_ = c.NoContent(http.StatusNotModified)
		return false
	}
	return true
}

// etagMatches implements the weak If-None-Match comparison: the header
// holds a comma-separated list of (possibly weak) tag literals, and a
// bare asterisk matches any current representation.
func etagMatches(header, want string) bool {
	if header == "*" {
		return true
	}
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		if part == want || strings.TrimPrefix(part, "W/") == want {
			return true
		}
	}
	return false
}

// avatarDigest is the placeholder's strong tag: a hash of the SVG
// itself, so the tag follows the bytes (initials and name edits
// included).
func avatarDigest(payload []byte) string {
	return crypto.Digest256(payload)
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

// processAvatar converts any accepted image into a centered 256x256
// JPEG: the decoder validates the image and its dimensions, the
// thumbnail crops from the center, and the JPEG re-encode drops
// metadata (EXIF) and normalizes every avatar to one format. GIFs are
// treated as static (first frame).
func processAvatar(payload []byte) ([]byte, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(payload))
	if err != nil {
		return nil, errors.New("the file is not a decodable image")
	}
	if cfg.Width > avatarMaxDimension || cfg.Height > avatarMaxDimension {
		return nil, errors.New("the image dimensions exceed the limit")
	}

	src, err := imaging.Decode(bytes.NewReader(payload))
	if err != nil {
		return nil, errors.New("the file is not a decodable image")
	}
	var img image.Image = imaging.Thumbnail(src, avatarSize, avatarSize, imaging.Lanczos)

	// JPEG has no alpha channel: opaque sources encode as-is, sources
	// with transparency are composed over white first.
	if opaque, ok := img.(interface{ Opaque() bool }); !ok || !opaque.Opaque() {
		bg := image.NewRGBA(image.Rect(0, 0, avatarSize, avatarSize))
		draw.Draw(bg, bg.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
		draw.Draw(bg, bg.Bounds(), img, image.Point{}, draw.Over)
		img = bg
	}

	var out bytes.Buffer
	if err := imaging.Encode(&out, img, imaging.JPEG, imaging.JPEGQuality(avatarJPEGQuality)); err != nil {
		return nil, errors.New("the image could not be processed")
	}
	return out.Bytes(), nil
}
