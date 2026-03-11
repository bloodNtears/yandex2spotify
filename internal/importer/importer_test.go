package importer

import (
	"testing"

	"github.com/bloodNtears/yandex2spotify/internal/yandex"
)

func TestTrackCacheKey_WithAlbum(t *testing.T) {
	track := yandex.Track{
		ID:     "12345",
		Albums: []yandex.Album{{ID: 678}},
	}
	got := trackCacheKey(track)
	want := "12345:678"
	if got != want {
		t.Errorf("trackCacheKey with album: got %q, want %q", got, want)
	}
}

func TestTrackCacheKey_WithoutAlbum(t *testing.T) {
	track := yandex.Track{ID: "12345"}
	got := trackCacheKey(track)
	want := "12345"
	if got != want {
		t.Errorf("trackCacheKey without album: got %q, want %q", got, want)
	}
}

func TestTrackCacheKey_MultipleAlbums(t *testing.T) {
	track := yandex.Track{
		ID:     "99",
		Albums: []yandex.Album{{ID: 10}, {ID: 20}},
	}
	got := trackCacheKey(track)
	want := "99:10"
	if got != want {
		t.Errorf("trackCacheKey uses first album: got %q, want %q", got, want)
	}
}

func TestFormatTrackName_SingleArtist(t *testing.T) {
	track := yandex.Track{
		Title:   "Song",
		Artists: []yandex.Artist{{Name: "Artist1"}},
	}
	got := formatTrackName(track)
	want := "Artist1 - Song"
	if got != want {
		t.Errorf("formatTrackName single artist: got %q, want %q", got, want)
	}
}

func TestFormatTrackName_MultipleArtists(t *testing.T) {
	track := yandex.Track{
		Title:   "Collab",
		Artists: []yandex.Artist{{Name: "A"}, {Name: "B"}, {Name: "C"}},
	}
	got := formatTrackName(track)
	want := "A, B, C - Collab"
	if got != want {
		t.Errorf("formatTrackName multiple artists: got %q, want %q", got, want)
	}
}

func TestFormatTrackName_NoArtists(t *testing.T) {
	track := yandex.Track{Title: "Lonely"}
	got := formatTrackName(track)
	want := " - Lonely"
	if got != want {
		t.Errorf("formatTrackName no artists: got %q, want %q", got, want)
	}
}

func TestBuildTrackQuery(t *testing.T) {
	track := yandex.Track{
		Title:   "MyTrack",
		Artists: []yandex.Artist{{Name: "TheArtist"}},
	}
	got := buildTrackQuery(track)
	want := "TheArtist MyTrack"
	if got != want {
		t.Errorf("buildTrackQuery: got %q, want %q", got, want)
	}
}

func TestBuildTrackQuery_MultipleArtists(t *testing.T) {
	track := yandex.Track{
		Title:   "Duo",
		Artists: []yandex.Artist{{Name: "X"}, {Name: "Y"}},
	}
	got := buildTrackQuery(track)
	want := "X, Y Duo"
	if got != want {
		t.Errorf("buildTrackQuery multi: got %q, want %q", got, want)
	}
}

func TestFormatAlbumName_WithArtists(t *testing.T) {
	album := yandex.Album{
		Title:   "AlbumTitle",
		Artists: []yandex.Artist{{Name: "Band"}},
	}
	got := formatAlbumName(album)
	want := "Band - AlbumTitle"
	if got != want {
		t.Errorf("formatAlbumName with artists: got %q, want %q", got, want)
	}
}

func TestFormatAlbumName_MultipleArtists(t *testing.T) {
	album := yandex.Album{
		Title:   "Joint",
		Artists: []yandex.Artist{{Name: "A"}, {Name: "B"}},
	}
	got := formatAlbumName(album)
	want := "A, B - Joint"
	if got != want {
		t.Errorf("formatAlbumName multiple artists: got %q, want %q", got, want)
	}
}

func TestFormatAlbumName_NoArtists(t *testing.T) {
	album := yandex.Album{Title: "Solo"}
	got := formatAlbumName(album)
	want := "Solo"
	if got != want {
		t.Errorf("formatAlbumName no artists: got %q, want %q", got, want)
	}
}

func TestBuildAlbumQuery(t *testing.T) {
	album := yandex.Album{
		Title:   "Record",
		Artists: []yandex.Artist{{Name: "Producer"}},
	}
	got := buildAlbumQuery(album)
	want := `album:"Record" artist:"Producer"`
	if got != want {
		t.Errorf("buildAlbumQuery: got %q, want %q", got, want)
	}
}

func TestChunkIDs_ExactMultiple(t *testing.T) {
	ids := []int{1, 2, 3, 4, 5, 6}
	chunks := chunkIDs(ids, 3)
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(chunks))
	}
	assertSliceEqual(t, chunks[0], []int{1, 2, 3})
	assertSliceEqual(t, chunks[1], []int{4, 5, 6})
}

func TestChunkIDs_Remainder(t *testing.T) {
	ids := []int{1, 2, 3, 4, 5}
	chunks := chunkIDs(ids, 2)
	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(chunks))
	}
	assertSliceEqual(t, chunks[0], []int{1, 2})
	assertSliceEqual(t, chunks[1], []int{3, 4})
	assertSliceEqual(t, chunks[2], []int{5})
}

func TestChunkIDs_SingleElement(t *testing.T) {
	ids := []int{42}
	chunks := chunkIDs(ids, 10)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	assertSliceEqual(t, chunks[0], []int{42})
}

func TestChunkIDs_Empty(t *testing.T) {
	chunks := chunkIDs([]int{}, 5)
	if len(chunks) != 0 {
		t.Fatalf("expected 0 chunks for empty slice, got %d", len(chunks))
	}
}

func TestChunkIDs_ChunkLargerThanSlice(t *testing.T) {
	ids := []int{1, 2, 3}
	chunks := chunkIDs(ids, 100)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	assertSliceEqual(t, chunks[0], []int{1, 2, 3})
}

func TestChunkIDs_SizeOne(t *testing.T) {
	ids := []string{"a", "b", "c"}
	chunks := chunkIDs(ids, 1)
	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(chunks))
	}
	for i, ch := range chunks {
		if len(ch) != 1 || ch[0] != ids[i] {
			t.Errorf("chunk %d: got %v, want [%v]", i, ch, ids[i])
		}
	}
}

func assertSliceEqual[T comparable](t *testing.T, got, want []T) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("length mismatch: got %d, want %d", len(got), len(want))
		return
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("index %d: got %v, want %v", i, got[i], want[i])
		}
	}
}
