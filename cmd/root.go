package cmd

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/bloodNtears/yandex2spotify/internal/importer"
	"github.com/bloodNtears/yandex2spotify/internal/yandex"
	"github.com/spf13/cobra"
	"github.com/zmb3/spotify/v2"
	spotifyauth "github.com/zmb3/spotify/v2/auth"
	"golang.org/x/oauth2"
)

const redirectURI = "https://open.spotify.com"

var (
	spotifyID     string
	spotifySecret string
	yandexToken   string
	ignoreItems   []string
	timeout       float64
)

var rootCmd = &cobra.Command{
	Use:   "yandex2spotify",
	Short: "Import music from Yandex Music to Spotify",
	Long:  "A CLI tool that imports liked tracks, playlists, albums, and artists from Yandex Music to Spotify.",
	RunE:  run,
}

func init() {
	rootCmd.Flags().StringVar(&spotifyID, "spotify-id", "", "Spotify application Client ID (required)")
	rootCmd.Flags().StringVar(&spotifySecret, "spotify-secret", "", "Spotify application Client Secret (required)")
	rootCmd.Flags().StringVarP(&yandexToken, "yandex-token", "t", "", "Yandex Music OAuth token (required)")
	rootCmd.Flags().StringSliceVarP(&ignoreItems, "ignore", "i", nil, "Items to skip: likes,playlists,albums,artists")
	rootCmd.Flags().Float64Var(&timeout, "timeout", 10, "HTTP request timeout in seconds")

	rootCmd.MarkFlagRequired("spotify-id")
	rootCmd.MarkFlagRequired("spotify-secret")
	rootCmd.MarkFlagRequired("yandex-token")
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func run(cmd *cobra.Command, args []string) error {
	ignore := make(map[string]bool)
	for _, item := range ignoreItems {
		ignore[item] = true
	}

	yc := yandex.NewClient(yandexToken, time.Duration(timeout)*time.Second)

	sc, err := authenticateSpotify(cmd.Context())
	if err != nil {
		return fmt.Errorf("spotify auth: %w", err)
	}

	imp, err := importer.New(yc, sc)
	if err != nil {
		return fmt.Errorf("init importer: %w", err)
	}

	imp.ImportAll(cmd.Context(), ignore)
	return nil
}

func authenticateSpotify(ctx context.Context) (*spotify.Client, error) {
	auth := spotifyauth.New(
		spotifyauth.WithClientID(spotifyID),
		spotifyauth.WithClientSecret(spotifySecret),
		spotifyauth.WithRedirectURL(redirectURI),
		spotifyauth.WithScopes(
			spotifyauth.ScopePlaylistModifyPublic,
			spotifyauth.ScopeUserLibraryModify,
			spotifyauth.ScopeUserFollowModify,
		),
	)

	state := fmt.Sprintf("yandex2spotify-%d", time.Now().UnixNano())

	authURL := auth.AuthURL(state)
	log.Println("Please log in to Spotify by visiting the following URL in your browser:")
	fmt.Println()
	fmt.Println(authURL)
	fmt.Println()
	log.Println("After authorizing, you will be redirected to open.spotify.com.")
	log.Println("Copy the full URL from your browser's address bar and paste it below:")
	fmt.Print("\nEnter the URL you were redirected to: ")

	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		return nil, fmt.Errorf("failed to read redirect URL")
	}
	redirectedURL := strings.TrimSpace(scanner.Text())

	if redirectedURL == "" {
		return nil, fmt.Errorf("empty redirect URL")
	}

	parsed, err := url.Parse(redirectedURL)
	if err != nil {
		return nil, fmt.Errorf("parse redirect URL: %w", err)
	}

	code := parsed.Query().Get("code")
	if code == "" {
		errMsg := parsed.Query().Get("error")
		if errMsg != "" {
			return nil, fmt.Errorf("spotify authorization denied: %s", errMsg)
		}
		return nil, fmt.Errorf("no authorization code found in URL")
	}

	returnedState := parsed.Query().Get("state")
	if returnedState != state {
		return nil, fmt.Errorf("state mismatch: possible CSRF attack")
	}

	oauthCfg := &oauth2.Config{
		ClientID:     spotifyID,
		ClientSecret: spotifySecret,
		RedirectURL:  redirectURI,
		Endpoint: oauth2.Endpoint{
			TokenURL: "https://accounts.spotify.com/api/token",
		},
	}

	tok, err := oauthCfg.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("exchange authorization code: %w", err)
	}

	client := spotify.New(auth.Client(ctx, tok), spotify.WithRetry(true))
	log.Println("Spotify authentication successful!")
	return client, nil
}
