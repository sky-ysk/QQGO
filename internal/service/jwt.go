package service

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/qqgo/server/internal/config"
)

var (
	ErrTokenExpired   = errors.New("token expired")
	ErrInvalidToken   = errors.New("invalid token")
	jwtSecret         []byte
	jwtAccessTTL      int
	jwtRefreshTTLDays int
)

func InitJWT(cfg config.JWTConfig) {
	jwtAccessTTL = cfg.AccessTTL
	jwtRefreshTTLDays = cfg.RefreshTTLDays
	if cfg.Secret == "" {
		b := make([]byte, 32)
		rand.Read(b)
		jwtSecret = b
	} else {
		jwtSecret = []byte(cfg.Secret)
	}
}

type jwtClaims struct {
	QQ int64 `json:"qq"`
	jwt.RegisteredClaims
}

func GenerateAccessToken(qq int64) (string, error) {
	claims := jwtClaims{
		QQ: qq,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(jwtAccessTTL) * time.Second)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecret)
}

func ValidateAccessToken(tokenString string) (int64, error) {
	token, err := jwt.ParseWithClaims(tokenString, &jwtClaims{}, func(token *jwt.Token) (interface{}, error) {
		return jwtSecret, nil
	})
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return 0, ErrTokenExpired
		}
		return 0, ErrInvalidToken
	}
	claims, ok := token.Claims.(*jwtClaims)
	if !ok || !token.Valid {
		return 0, ErrInvalidToken
	}
	return claims.QQ, nil
}

func GenerateRefreshToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func GetRefreshTTLDays() int {
	return jwtRefreshTTLDays
}
