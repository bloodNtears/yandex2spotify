package spotify

type SearchResult struct {
	Tracks *TracksPage `json:"tracks,omitempty"`
	Albums *AlbumsPage `json:"albums,omitempty"`
}

type TracksPage struct {
	Items []TrackItem `json:"items"`
}

type AlbumsPage struct {
	Items []AlbumItem `json:"items"`
}

type TrackItem struct {
	ID string `json:"id"`
}

type AlbumItem struct {
	ID string `json:"id"`
}
