package api

import (
	"context"
	"log"
	"net/http"
	"strings"
	"time"

	"chat-app/internal/application/api_helpers" // For RespondWithError
	"chat-app/internal/infrastructure/auth"    // For JWTService and ContextKeys
)

// LoggingMiddleware logs incoming HTTP requests.
func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		// Log request (method, URI)
		log.Printf("Request: %s %s", r.Method, r.RequestURI)

		// Call the next handler
		next.ServeHTTP(w, r)

		// Log time taken
		log.Printf("Response: %s %s processed in %v", r.Method, r.RequestURI, time.Since(start))
	})
}

// CORSMiddleware sets permissive CORS headers for development.
// For production, specify allowed origins, methods, and headers.
func CORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Set CORS headers
		w.Header().Set("Access-Control-Allow-Origin", "*") // Allow any origin for development
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE, PATCH")
		w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")
		w.Header().Set("Access-Control-Allow-Credentials", "true")

		// Handle preflight requests (OPTIONS)
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		// Call the next handler
		next.ServeHTTP(w, r)
	})
}

// AuthMiddlewareFactory creates an AuthMiddleware that uses the provided JWTService.
func AuthMiddlewareFactory(jwtService *auth.JWTService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				log.Println("AuthMiddleware: Authorization header missing.")
				api_helpers.RespondWithError(w, http.StatusUnauthorized, "Authorization header required")
				return
			}

			parts := strings.Split(authHeader, " ")
			if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
				log.Println("AuthMiddleware: Invalid Authorization header format (not Bearer).")
				api_helpers.RespondWithError(w, http.StatusUnauthorized, "Invalid Authorization header format")
				return
			}
			tokenString := parts[1]

			claims, err := jwtService.ValidateToken(tokenString)
			if err != nil {
				log.Printf("AuthMiddleware: Invalid token: %v", err)
				api_helpers.RespondWithError(w, http.StatusUnauthorized, "Invalid or expired token")
				return
			}

			// Check if it's an access token
			if claims.Type != auth.TokenTypeAccess {
				log.Printf("AuthMiddleware: Invalid token type: expected '%s', got '%s'", auth.TokenTypeAccess, claims.Type)
				api_helpers.RespondWithError(w, http.StatusUnauthorized, "Invalid token type: not an access token")
				return
			}

			log.Printf("AuthMiddleware: Token valid for UserID: %s, Username: %s. Path: %s %s", claims.UserID, claims.Username, r.Method, r.RequestURI)

			// Store user information in context
			ctx := context.WithValue(r.Context(), auth.UserIDKey, claims.UserID)
			ctx = context.WithValue(ctx, auth.UsernameKey, claims.Username)
			ctx = context.WithValue(ctx, auth.UserClaimsKey, claims) // Store all claims if needed

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
