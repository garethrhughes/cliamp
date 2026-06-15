package playlist

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dhowden/tag"
)

func TestFileURL(t *testing.T) {
	got := fileURL("/tmp/My Music/cover art.jpg")
	want := "file:///tmp/My%20Music/cover%20art.jpg"
	if got != want {
		t.Errorf("fileURL = %q, want %q", got, want)
	}
}

func TestLocalArtURL_SiblingCover(t *testing.T) {
	dir := t.TempDir()
	cover := filepath.Join(dir, "cover.jpg")
	if err := os.WriteFile(cover, []byte("img"), 0o644); err != nil {
		t.Fatal(err)
	}
	audio := filepath.Join(dir, "01 - song.flac")

	// Sibling cover is found before any tag metadata is needed, so m can be nil.
	got := localArtURL(audio, nil)
	if got != fileURL(cover) {
		t.Errorf("localArtURL = %q, want sibling cover %q", got, fileURL(cover))
	}
}

func TestCacheEmbeddedArt(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)

	pic := &tag.Picture{Data: []byte("PNGDATA"), Ext: "png"}
	audio := "/music/Album/01.flac"

	got := cacheEmbeddedArt(audio, "Album", "Artist", pic)
	if got == "" {
		t.Fatal("cacheEmbeddedArt returned empty")
	}
	if !strings.HasSuffix(got, ".png") {
		t.Errorf("cache path %q should keep the picture extension", got)
	}
	data, err := os.ReadFile(got)
	if err != nil || string(data) != "PNGDATA" {
		t.Errorf("cached file content = %q (err %v), want PNGDATA", data, err)
	}

	// A second call for the same album reuses the same cached file.
	if again := cacheEmbeddedArt(audio, "Album", "Artist", pic); again != got {
		t.Errorf("second call = %q, want same path %q", again, got)
	}
}
