// Package auth stores a per-user GitHub token in a signed session.
package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

const (
	sessionToken = "gh_token"
	sessionLogin = "gh_login"
)

func Token(c *gin.Context) string {
	tok, _ := sessions.Default(c).Get(sessionToken).(string)
	return tok
}

func CurrentLogin(c *gin.Context) string {
	login, _ := sessions.Default(c).Get(sessionLogin).(string)
	return login
}

func SetSession(c *gin.Context, token, login string) error {
	sess := sessions.Default(c)
	sess.Set(sessionToken, token)
	sess.Set(sessionLogin, login)
	return sess.Save()
}

func Clear(c *gin.Context) {
	sess := sessions.Default(c)
	sess.Clear()
	_ = sess.Save()
}

func RequireToken(c *gin.Context) {
	if Token(c) == "" {
		c.Redirect(http.StatusFound, "/login")
		c.Abort()
		return
	}
	c.Next()
}

func Validate(ctx context.Context, token string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/user", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token ditolak GitHub: %s", resp.Status)
	}

	var body struct {
		Login string `json:"login"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", err
	}
	return body.Login, nil
}
