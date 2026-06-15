package playlist

import (
	"crypto/sha1"
	"encoding/hex"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/dhowden/tag"
)

// siblingCoverNames are common cover-art filenames placed alongside audio files.
var siblingCoverNames = []string{
	"cover.jpg", "cover.jpeg", "cover.png",
	"folder.jpg", "folder.jpeg", "folder.png",
	"front.jpg", "front.png", "album.jpg", "albumart.jpg",
}

// readTags reads embedded metadata (ID3v2, Vorbis comments, MP4 atoms) from
// a local audio file and returns a Track. Falls back to filename parsing if
// tag reading fails or the tags contain no title.
func readTags(path string) Track {
	f, err := os.Open(path)
	if err != nil {
		return TrackFromFilename(path)
	}
	defer f.Close()

	m, err := tag.ReadFrom(f)
	if err != nil || m == nil || strings.TrimSpace(m.Title()) == "" {
		return TrackFromFilename(path)
	}

	t := Track{
		Path:   path,
		Title:  sanitizeTag(strings.TrimSpace(m.Title())),
		Artist: sanitizeTag(strings.TrimSpace(m.Artist())),
		Album:  sanitizeTag(strings.TrimSpace(m.Album())),
		Genre:  sanitizeTag(strings.TrimSpace(m.Genre())),
		Year:   m.Year(),
	}
	trackNum, _ := m.Track()
	t.TrackNumber = trackNum
	t.ArtURL = localArtURL(path, m)
	return t
}

// localArtURL resolves cover art for a local file as a file:// URL. It prefers a
// sibling cover file (no disk writes); otherwise it extracts the embedded
// picture once per album into the user cache directory. Returns "" if none.
func localArtURL(audioPath string, m tag.Metadata) string {
	dir := filepath.Dir(audioPath)
	for _, name := range siblingCoverNames {
		cand := filepath.Join(dir, name)
		if fi, err := os.Stat(cand); err == nil && !fi.IsDir() {
			return fileURL(cand)
		}
	}
	if pic := m.Picture(); pic != nil && len(pic.Data) > 0 {
		if cached := cacheEmbeddedArt(audioPath, m.Album(), m.Artist(), pic); cached != "" {
			return fileURL(cached)
		}
	}
	return ""
}

// cacheEmbeddedArt writes embedded cover bytes to the user cache directory and
// returns the file path. The cache key is album+artist+directory, so every
// track in an album reuses one extracted file. Returns "" on any failure.
func cacheEmbeddedArt(audioPath, album, artist string, pic *tag.Picture) string {
	base, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	artDir := filepath.Join(base, "cliamp", "art")

	sum := sha1.Sum([]byte(album + "\x00" + artist + "\x00" + filepath.Dir(audioPath)))
	ext := strings.TrimPrefix(pic.Ext, ".")
	if ext == "" {
		ext = "jpg"
	}
	dest := filepath.Join(artDir, hex.EncodeToString(sum[:])+"."+ext)

	if fi, err := os.Stat(dest); err == nil && fi.Size() > 0 {
		return dest
	}
	if err := os.MkdirAll(artDir, 0o755); err != nil {
		return ""
	}
	tmp := dest + ".tmp"
	if err := os.WriteFile(tmp, pic.Data, 0o644); err != nil {
		return ""
	}
	if err := os.Rename(tmp, dest); err != nil {
		os.Remove(tmp)
		return ""
	}
	return dest
}

// fileURL converts a filesystem path to a file:// URL with proper escaping.
func fileURL(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	return (&url.URL{Scheme: "file", Path: abs}).String()
}

// TrackFromFilename creates a Track by parsing "Artist - Title" from the
// filename, or using the bare filename as the title.
func TrackFromFilename(path string) Track {
	base := filepath.Base(path)
	name := sanitizeTag(strings.TrimSuffix(base, filepath.Ext(base)))
	parts := strings.SplitN(name, " - ", 2)
	if len(parts) == 2 {
		return Track{Path: path, Artist: strings.TrimSpace(parts[0]), Title: strings.TrimSpace(parts[1])}
	}
	return Track{Path: path, Title: name}
}
