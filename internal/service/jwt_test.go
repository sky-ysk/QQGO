package service

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/qqgo/server/internal/config"
)

func TestGenerateAccessToken(t *testing.T) {
	InitJWT(config.JWTConfig{Secret: "test-secret", AccessTTL: 900, RefreshTTLDays: 7})

	token, err := GenerateAccessToken(10001)
	if err != nil {
		t.Fatalf("generate failed: %v", err)
	}
	if token == "" {
		t.Fatal("token should not be empty")
	}

	qq, err := ValidateAccessToken(token)
	if err != nil {
		t.Fatalf("validate failed: %v", err)
	}
	if qq != 10001 {
		t.Fatalf("expected qq 10001, got %d", qq)
	}
}

func TestValidateAccessTokenExpired(t *testing.T) {
	InitJWT(config.JWTConfig{Secret: "test-secret", AccessTTL: 1, RefreshTTLDays: 7})

	claims := jwtClaims{
		QQ: 10001,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Second)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, _ := token.SignedString(jwtSecret)

	_, err := ValidateAccessToken(tokenString)
	if err != ErrTokenExpired {
		t.Fatalf("expected ErrTokenExpired, got: %v", err)
	}
}

func TestValidateAccessTokenInvalid(t *testing.T) {
	InitJWT(config.JWTConfig{Secret: "test-secret", AccessTTL: 900, RefreshTTLDays: 7})

	_, err := ValidateAccessToken("not-a-jwt-token")
	if err != ErrInvalidToken {
		t.Fatalf("expected ErrInvalidToken, got: %v", err)
	}
}

func TestValidateAccessTokenWrongSecret(t *testing.T) {
	InitJWT(config.JWTConfig{Secret: "secret-a", AccessTTL: 900, RefreshTTLDays: 7})
	token, _ := GenerateAccessToken(10001)

	InitJWT(config.JWTConfig{Secret: "secret-b", AccessTTL: 900, RefreshTTLDays: 7})
	_, err := ValidateAccessToken(token)
	if err != ErrInvalidToken {
		t.Fatalf("expected ErrInvalidToken with wrong secret, got: %v", err)
	}
}

func TestGenerateRefreshToken(t *testing.T) {
	token1 := GenerateRefreshToken()
	token2 := GenerateRefreshToken()
	if token1 == token2 {
		t.Fatal("refresh tokens should be unique")
	}
	if len(token1) != 64 {
		t.Fatalf("expected 64 chars, got %d", len(token1))
	}
}
