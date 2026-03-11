package cmd

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bloodNtears/yandex2spotify/internal/cache"
	"github.com/bloodNtears/yandex2spotify/internal/importer"
	"github.com/bloodNtears/yandex2spotify/internal/spotify"
	"github.com/bloodNtears/yandex2spotify/internal/yandex"
	"github.com/spf13/cobra"
	"golang.org/x/oauth2"
)

const redirectURI = "https://open.spotify.com"

var (
	spotifyID     string
	spotifySecret string
	yandexToken   string
	ignoreItems   []string
	timeout       float64
	cacheFile     string
)

var rootCmd = &cobra.Command{
	Use:   "yandex2spotify",
	Short: "Import music from Yandex Music to Spotify",
	Long:  "A CLI tool that imports liked tracks, playlists, and albums from Yandex Music to Spotify.",
	RunE:  run,
}

func defaultCachePath() string {
	return filepath.Join("tmp", "cache", "cache.json")
}

func init() {
	rootCmd.Flags().StringVar(&spotifyID, "spotify-id", "", "Spotify application Client ID (required)")
	rootCmd.Flags().StringVar(&spotifySecret, "spotify-secret", "", "Spotify application Client Secret (required)")
	rootCmd.Flags().StringVarP(&yandexToken, "yandex-token", "t", "", "Yandex Music OAuth token (required)")
	rootCmd.Flags().StringSliceVarP(&ignoreItems, "ignore", "i", nil, "Items to skip: likes,playlists,albums")
	rootCmd.Flags().Float64Var(&timeout, "timeout", 10, "HTTP request timeout in seconds")
	rootCmd.Flags().StringVar(&cacheFile, "cache-file", defaultCachePath(), "Path to the search results cache file")

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

	c, err := cache.Load(cacheFile)
	if err != nil {
		return fmt.Errorf("load cache: %w", err)
	}
	log.Printf("Cache file: %s", cacheFile)

	imp, err := importer.New(yc, sc, c)
	if err != nil {
		return fmt.Errorf("init importer: %w", err)
	}

	ctx, cancel := context.WithTimeout(cmd.Context(), 48*time.Hour)
	defer cancel()

	imp.ImportAll(ctx, ignore)
	return nil
}

var spotifyScopes = []string{
	"playlist-modify-public",
	"playlist-modify-private",
	"playlist-read-private",
	"playlist-read-collaborative",
	"user-library-modify",
}

func authenticateSpotify(ctx context.Context) (*spotify.Client, error) {
	state := fmt.Sprintf("yandex2spotify-%d", time.Now().UnixNano())

	params := url.Values{}
	params.Set("client_id", spotifyID)
	params.Set("response_type", "code")
	params.Set("redirect_uri", redirectURI)
	params.Set("scope", strings.Join(spotifyScopes, " "))
	params.Set("state", state)

	authURL := "https://accounts.spotify.com/authorize?" + params.Encode()

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

	httpClient := oauthCfg.Client(ctx, tok)
	client := spotify.NewClient(httpClient)

	log.Println("Spotify authentication successful!")
	return client, nil
}
