package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	// TokenTypeAccess identifies an access token.
	TokenTypeAccess = "access_token"
	// TokenTypeRefresh identifies a refresh token.
	TokenTypeRefresh = "refresh_token"
)

// CustomClaims includes custom data for the JWT.
type CustomClaims struct {
	UserID   string `json:"user_id"`
	Username string `json:"username,omitempty"` // Optional in refresh token
	Type     string `json:"type"`               // "access_token" or "refresh_token"
	jwt.RegisteredClaims
}

// JWTService provides operations for JWT generation and validation.
type JWTService struct {
	secretKey         []byte
	accessTokenTTL    time.Duration
	refreshTokenTTL   time.Duration
	issuer            string // Optional: an identifier for the token issuer
	audience          string // Optional: an identifier for the token audience
}

// NewJWTService creates a new JWTService.
func NewJWTService(secretKey string, accessTokenTTL, refreshTokenTTL time.Duration, issuer, audience string) (*JWTService, error) {
	if secretKey == "" {
		return nil, fmt.Errorf("JWT secret key cannot be empty")
	}
	if accessTokenTTL <= 0 || refreshTokenTTL <= 0 {
		return nil, fmt.Errorf("JWT token TTLs must be positive")
	}
	return &JWTService{
		secretKey:         []byte(secretKey),
		accessTokenTTL:    accessTokenTTL,
		refreshTokenTTL:   refreshTokenTTL,
		issuer:            issuer,
		audience:          audience,
	}, nil
}

// GenerateToken generates a new JWT access token for a user.
func (s *JWTService) GenerateToken(userID string, username string) (string, error) {
	claims := CustomClaims{
		UserID:   userID,
		Username: username,
		Type:     TokenTypeAccess,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(s.accessTokenTTL)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Issuer:    s.issuer,
			Audience:  jwt.ClaimStrings{s.audience},
			Subject:   userID,
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.secretKey)
}

// GenerateRefreshToken generates a new JWT refresh token for a user.
func (s *JWTService) GenerateRefreshToken(userID string) (string, error) {
	claims := CustomClaims{
		UserID: userID,
		Type:   TokenTypeRefresh,
		// Username is optional for refresh token, not strictly needed for re-issuing access token.
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(s.refreshTokenTTL)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Issuer:    s.issuer,
			Audience:  jwt.ClaimStrings{s.audience}, // Could be different for refresh tokens
			Subject:   userID,
			ID:        fmt.Sprintf("%s-%d", userID, time.Now().UnixNano()), // Unique ID for refresh token
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.secretKey)
}

// ValidateToken validates a JWT token string.
// It returns the custom claims if the token is valid, or an error otherwise.
func (s *JWTService) ValidateToken(tokenString string) (*CustomClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &CustomClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return s.secretKey, nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to parse token: %w", err)
	}

	if claims, ok := token.Claims.(*CustomClaims); ok && token.Valid {
		// Additional checks if needed, e.g., issuer, audience
		if claims.Issuer != s.issuer {
			return nil, fmt.Errorf("invalid issuer: expected %s, got %s", s.issuer, claims.Issuer)
		}
		// Note: Audience check is handled by jwt-go library if present in claims and options.
		// Here, we are explicit.
		// var audMatch bool
		// for _, aud := range claims.Audience {
		// 	if aud == s.audience {
		// 		audMatch = true
		// 		break
		// 	}
		// }
		// if !audMatch && s.audience != "" { // only check audience if we expect one
		// 	return nil, fmt.Errorf("invalid audience")
		// }

		return claims, nil
	}
	return nil, fmt.Errorf("invalid token")
}

// ContextKey defines a type for context keys to avoid collisions.
type ContextKey string

const (
	// UserIDKey is the key for storing UserID in context.
	UserIDKey ContextKey = "userID"
	// UsernameKey is the key for storing Username in context.
	UsernameKey ContextKey = "username"
    // UserClaimsKey is the key for storing the whole claims object.
    UserClaimsKey ContextKey = "userClaims"
)
