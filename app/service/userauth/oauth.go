package userauth

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	userModel "meta-api/app/model/user"
	"meta-api/common/cachekey"
	"meta-api/common/env"
	"meta-api/common/idutil"
	"meta-api/common/types"
	"meta-api/common/utils"
)

const (
	oauthStateTTL       = 5 * time.Minute
	oauthRequestTimeout = 15 * time.Second
)

type oauthStatePayload struct {
	Provider     string `json:"provider"`
	RedirectPath string `json:"redirectPath"`
}

type oauthProviderConfig struct {
	Provider     string
	ClientID     string
	ClientSecret string
	RedirectURI  string
	AuthURL      string
	TokenURL     string
	UserInfoURL  string
	Scopes       []string
}

func (s *userAuthService) BuildOAuthLoginURL(ctx context.Context, request *types.OAuthLoginRequest) (string, error) {
	provider, err := s.loadOAuthProviderConfig(request.Provider)
	if err != nil {
		return "", err
	}

	redirectPath, err := normalizeRedirectPath(request.Redirect)
	if err != nil {
		return "", err
	}
	state, err := generateOAuthState()
	if err != nil {
		return "", fmt.Errorf("failed to generate oauth state: %w", err)
	}

	payload, err := json.Marshal(oauthStatePayload{
		Provider:     provider.Provider,
		RedirectPath: redirectPath,
	})
	if err != nil {
		return "", fmt.Errorf("failed to marshal oauth state: %w", err)
	}
	if err = s.redis.Set(ctx, cachekey.UserOAuthState(state).String(), string(payload), oauthStateTTL).Err(); err != nil {
		return "", fmt.Errorf("failed to store oauth state: %w", err)
	}

	query := url.Values{}
	query.Set("client_id", provider.ClientID)
	query.Set("redirect_uri", provider.RedirectURI)
	query.Set("response_type", "code")
	query.Set("scope", strings.Join(provider.Scopes, " "))
	query.Set("state", state)
	if provider.Provider == "google" {
		query.Set("access_type", "online")
	}
	return provider.AuthURL + "?" + query.Encode(), nil
}

func (s *userAuthService) HandleOAuthCallback(ctx context.Context,
	request *types.OAuthCallbackRequest) (*types.OAuthCallbackResponse, string, error) {

	statePayload, err := s.consumeOAuthState(ctx, request.State)
	if err != nil {
		return nil, "", err
	}
	if statePayload.Provider != request.Provider {
		return nil, "", ErrInvalidOAuthState
	}

	provider, err := s.loadOAuthProviderConfig(request.Provider)
	if err != nil {
		return nil, "", err
	}
	accessToken, err := exchangeOAuthToken(ctx, provider, request.Code)
	if err != nil {
		s.logger.Error("failed to exchange oauth token", zap.String("provider", provider.Provider), zap.Error(err))
		return nil, "", ErrUserAuthFailed
	}

	oauthUser, err := fetchOAuthUser(ctx, provider, accessToken)
	if err != nil {
		s.logger.Error("failed to fetch oauth user", zap.String("provider", provider.Provider), zap.Error(err))
		return nil, "", ErrUserAuthFailed
	}

	now, err := nowInShanghai()
	if err != nil {
		return nil, "", err
	}
	userID, err := s.idGenerator.NextID()
	if err != nil {
		return nil, "", fmt.Errorf("failed to generate user id: %w", err)
	}
	var account *userModel.User
	for attempt := uint64(0); attempt < userHandleGenerateAttempts; attempt++ {
		handle, err := s.generateOAuthHandle(ctx, attempt)
		if err != nil {
			return nil, "", err
		}
		account, err = s.userModel.UpsertOAuthUser(ctx, &userModel.User{
			ID:             userID,
			Provider:       provider.Provider,
			ProviderUserID: oauthUser.ProviderUserID,
			DisplayName:    oauthUser.DisplayName,
			Handle:         handle,
			AvatarURL:      oauthUser.AvatarURL,
			ProfileURL:     oauthUser.ProfileURL,
			Email:          oauthUser.Email,
			SessionVersion: 1,
			CreateTime:     now,
			UpdateTime:     now,
		})
		if err == nil {
			break
		}
		if !isDuplicateUserHandleError(err) {
			return nil, "", err
		}
	}
	if account == nil {
		return nil, "", ErrUserHandleTaken
	}

	publicUser := toPublicUserInfo(account)
	token, err := utils.GenerateCommentUserToken(publicUser)
	if err != nil {
		return nil, "", err
	}

	return &types.OAuthCallbackResponse{
		User:         *publicUser,
		RedirectPath: statePayload.RedirectPath,
	}, token, nil
}

func (s *userAuthService) GetCurrentUser(ctx context.Context, userID string, sessionVersion int64) (*types.PublicUserInfo, error) {
	id, err := idutil.ParseID("userID", userID)
	if err != nil {
		return nil, ErrUserAuthFailed
	}
	account, err := s.userModel.GetUserByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if sessionVersion != account.SessionVersion {
		return nil, ErrUserSessionInvalid
	}
	return toPublicUserInfo(account), nil
}

func (s *userAuthService) consumeOAuthState(ctx context.Context, state string) (*oauthStatePayload, error) {
	key := cachekey.UserOAuthState(state).String()
	// OAuth state 是一次性凭证，必须原子读取并删除，避免两个并发回调都在
	// GET 与 DEL 的间隙内通过校验。
	raw, err := s.redis.GetDel(ctx, key).Result()
	if err != nil {
		return nil, ErrInvalidOAuthState
	}

	payload := new(oauthStatePayload)
	if err = json.Unmarshal([]byte(raw), payload); err != nil {
		return nil, ErrInvalidOAuthState
	}
	if payload.Provider == "" || payload.RedirectPath == "" {
		return nil, ErrInvalidOAuthState
	}
	return payload, nil
}

type normalizedOAuthUser struct {
	ProviderUserID string
	DisplayName    string
	Login          string
	AvatarURL      string
	ProfileURL     string
	Email          string
}

func (s *userAuthService) loadOAuthProviderConfig(provider string) (*oauthProviderConfig, error) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	providerConfig := s.config.OAuthProviderSnapshot(provider)
	clientSecret, err := utils.EnvOrFile(env.OAuthClientSecret(provider))
	if err != nil {
		return nil, err
	}
	config := &oauthProviderConfig{
		Provider:     provider,
		ClientID:     envOrConfig(env.OAuthClientID(provider), providerConfig.ClientID),
		ClientSecret: clientSecret,
		RedirectURI:  envOrConfig(env.OAuthRedirectURI(provider), providerConfig.RedirectURI),
	}
	switch provider {
	case "github":
		config.AuthURL = "https://github.com/login/oauth/authorize"
		config.TokenURL = "https://github.com/login/oauth/access_token"
		config.UserInfoURL = "https://api.github.com/user"
		config.Scopes = []string{"read:user", "user:email"}
	case "google":
		config.AuthURL = "https://accounts.google.com/o/oauth2/v2/auth"
		config.TokenURL = "https://oauth2.googleapis.com/token"
		config.UserInfoURL = "https://www.googleapis.com/oauth2/v3/userinfo"
		config.Scopes = []string{"openid", "profile", "email"}
	default:
		return nil, ErrOAuthProviderMissing
	}
	if config.ClientID == "" || config.ClientSecret == "" || config.RedirectURI == "" {
		return nil, ErrOAuthProviderMissing
	}
	return config, nil
}

func envOrConfig(envKey string, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(envKey)); value != "" {
		return value
	}
	return strings.TrimSpace(fallback)
}

func exchangeOAuthToken(ctx context.Context, provider *oauthProviderConfig, code string) (string, error) {
	form := url.Values{}
	form.Set("client_id", provider.ClientID)
	form.Set("client_secret", provider.ClientSecret)
	form.Set("code", code)
	form.Set("grant_type", "authorization_code")
	form.Set("redirect_uri", provider.RedirectURI)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, provider.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	body, err := doOAuthRequest(req)
	if err != nil {
		return "", err
	}
	var response struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
		Description string `json:"error_description"`
	}
	if err = json.Unmarshal(body, &response); err != nil {
		return "", err
	}
	if response.AccessToken == "" {
		return "", fmt.Errorf("empty access token: %s %s", response.Error, response.Description)
	}
	return response.AccessToken, nil
}

func fetchOAuthUser(ctx context.Context, provider *oauthProviderConfig, accessToken string) (*normalizedOAuthUser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, provider.UserInfoURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	body, err := doOAuthRequest(req)
	if err != nil {
		return nil, err
	}
	switch provider.Provider {
	case "github":
		return normalizeGitHubUser(body)
	case "google":
		return normalizeGoogleUser(body)
	default:
		return nil, ErrOAuthProviderMissing
	}
}

func doOAuthRequest(req *http.Request) ([]byte, error) {
	client := &http.Client{Timeout: oauthRequestTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("oauth request failed: status=%d body=%s", resp.StatusCode, string(bytes.TrimSpace(body)))
	}
	return body, nil
}

func normalizeGitHubUser(body []byte) (*normalizedOAuthUser, error) {
	var user struct {
		ID        int64  `json:"id"`
		Login     string `json:"login"`
		Name      string `json:"name"`
		AvatarURL string `json:"avatar_url"`
		HTMLURL   string `json:"html_url"`
		Email     string `json:"email"`
	}
	if err := json.Unmarshal(body, &user); err != nil {
		return nil, err
	}
	displayName := strings.TrimSpace(user.Name)
	if displayName == "" {
		displayName = strings.TrimSpace(user.Login)
	}
	if user.ID == 0 || displayName == "" {
		return nil, errors.New("invalid github user")
	}
	return &normalizedOAuthUser{
		ProviderUserID: strconv.FormatInt(user.ID, 10),
		DisplayName:    displayName,
		Login:          strings.TrimSpace(user.Login),
		AvatarURL:      user.AvatarURL,
		ProfileURL:     user.HTMLURL,
		Email:          user.Email,
	}, nil
}

func normalizeGoogleUser(body []byte) (*normalizedOAuthUser, error) {
	var user struct {
		Sub     string `json:"sub"`
		Name    string `json:"name"`
		Picture string `json:"picture"`
		Profile string `json:"profile"`
		Email   string `json:"email"`
	}
	if err := json.Unmarshal(body, &user); err != nil {
		return nil, err
	}
	displayName := strings.TrimSpace(user.Name)
	if user.Sub == "" || displayName == "" {
		return nil, errors.New("invalid google user")
	}
	return &normalizedOAuthUser{
		ProviderUserID: user.Sub,
		DisplayName:    displayName,
		AvatarURL:      user.Picture,
		ProfileURL:     user.Profile,
		Email:          user.Email,
	}, nil
}

func generateOAuthState() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func normalizeRedirectPath(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "/", nil
	}
	if !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") || strings.Contains(raw, "://") {
		return "", ErrInvalidOAuthRedirect
	}
	return raw, nil
}

func nowInShanghai() (time.Time, error) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to load location: %w", err)
	}
	return time.Now().In(loc), nil
}

func toPublicUserInfo(user *userModel.User) *types.PublicUserInfo {
	return &types.PublicUserInfo{
		ID:             strconv.FormatUint(user.ID, 10),
		Provider:       user.Provider,
		DisplayName:    user.DisplayName,
		Handle:         user.Handle,
		SessionVersion: user.SessionVersion,
		AvatarURL:      user.AvatarURL,
		ProfileURL:     user.ProfileURL,
	}
}
