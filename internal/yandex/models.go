package yandex

type responseBase struct {
	Error string `json:"error,omitempty"`
}

type Account struct {
	Login       string `json:"login,omitempty"`
	FullName    string `json:"fullName,omitempty"`
	DisplayName string `json:"displayName,omitempty"`
	UID         uint32 `json:"uid,omitempty"`
}

type AccountStatus struct {
	Account Account `json:"account"`
}

type accountStatusResp struct {
	responseBase
	Result AccountStatus `json:"result"`
}

type Artist struct {
	Name string `json:"name,omitempty"`
	ID   int    `json:"id,omitempty"`
}

type Album struct {
	Title   string   `json:"title,omitempty"`
	ID      int      `json:"id,omitempty"`
	Artists []Artist `json:"artists,omitempty"`
}

type Track struct {
	Title   string   `json:"title,omitempty"`
	ID      string   `json:"id,omitempty"`
	Artists []Artist `json:"artists,omitempty"`
	Albums  []Album  `json:"albums,omitempty"`
}

type TrackShort struct {
	ID    int    `json:"id,omitempty"`
	Track *Track `json:"track,omitempty"`
}

type Cover struct {
	Type string `json:"type,omitempty"`
	URI  string `json:"uri,omitempty"`
}

type Playlist struct {
	Title      string       `json:"title,omitempty"`
	Kind       int          `json:"kind,omitempty"`
	UID        uint32       `json:"uid,omitempty"`
	TrackCount int          `json:"trackCount,omitempty"`
	Tracks     []TrackShort `json:"tracks,omitempty"`
	Collective bool         `json:"collective,omitempty"`
	Cover      Cover        `json:"cover,omitempty"`
}

type playlistListResp struct {
	responseBase
	Result []Playlist `json:"result"`
}

type playlistResp struct {
	responseBase
	Result *Playlist `json:"result"`
}

type LikedTrack struct {
	ID      string `json:"id,omitempty"`
	AlbumID string `json:"albumId,omitempty"`
}

type likedTracksResp struct {
	responseBase
	Result struct {
		Library struct {
			Tracks []LikedTrack `json:"tracks,omitempty"`
		} `json:"library,omitempty"`
	} `json:"result"`
}

type LikedAlbum struct {
	ID int `json:"id"`
}

type likedAlbumsResp struct {
	responseBase
	Result []LikedAlbum `json:"result"`
}

type albumsResp struct {
	responseBase
	Result []Album `json:"result"`
}

type tracksResp struct {
	responseBase
	Result []Track `json:"result"`
}
