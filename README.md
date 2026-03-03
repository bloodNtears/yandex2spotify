# yandex2spotify

A Go CLI tool that imports liked tracks, playlists, albums, and artists from Yandex Music to Spotify.

Inspired by [yandex2spotify](https://github.com/MarshalX/yandex2spotify).

## Installation

Requires Go 1.21 or higher.

```bash
go install github.com/bloodNtears/yandex2spotify@latest
```

Or build from source:

```bash
git clone https://github.com/bloodNtears/yandex2spotify.git
cd yandex2spotify
go build -o yandex2spotify .
```

## Prerequisites

1. **Spotify OAuth application** — register at https://developer.spotify.com/dashboard and add `https://open.spotify.com` as a Redirect URI.

2. **Yandex Music OAuth token** — since there is no public OAuth scope for Yandex Music, you need to extract the token from a `music.yandex.ru` browser session (look for the `OAuth` token in request headers via browser DevTools).

## Usage

```bash
./yandex2spotify \
  --spotify-id <ID> \
  --spotify-secret <SECRET> \
  --yandex-token <TOKEN>
```

The tool will print a Spotify authorization URL. Open it in your browser, log in, and after authorizing you'll be redirected to `open.spotify.com`. Copy the full URL from your browser's address bar and paste it back into the terminal. The import will then begin automatically.

### Skipping item types

Use `--ignore` to skip certain item types:

```bash
# Import only playlists (skip likes, albums, artists)
yandex2spotify \
  --spotify-id <ID> --spotify-secret <SECRET> -t <TOKEN> \
  --ignore likes,albums,artists

# Import everything except artists
yandex2spotify \
  --spotify-id <ID> --spotify-secret <SECRET> -t <TOKEN> \
  --ignore artists
```

Valid values for `--ignore`: `likes`, `playlists`, `albums`, `artists`.

### All flags

| Flag               | Short | Description                     | Default    |
|--------------------|-------|---------------------------------|------------|
| `--spotify-id`     |       | Spotify Client ID               | (required) |
| `--spotify-secret` |       | Spotify Client Secret           | (required) |
| `--yandex-token`   | `-t`  | Yandex Music OAuth token        | (required) |
| `--ignore`         | `-i`  | Item types to skip              | (none)     |
| `--timeout`        |       | HTTP request timeout in seconds | `10`       |

## How it works

1. Authenticates with Yandex Music using the provided token
2. Authenticates with Spotify via OAuth2 (user pastes redirect URL from browser)
3. For each enabled item type:
   - **Likes**: fetches liked tracks, searches each on Spotify, saves matches to your library
   - **Playlists**: fetches playlists and their tracks, creates corresponding playlists on Spotify, adds matched tracks
   - **Albums**: fetches liked albums, searches on Spotify, saves matches to your library
   - **Artists**: fetches liked artists, searches on Spotify, follows matches
4. Prints a summary of items that could not be found on Spotify

## License

MIT
