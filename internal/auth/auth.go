package auth

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	jwt.RegisteredClaims
}

type Client struct {
	authPort string
}

func NewClient(authPort string) *Client {
	return &Client{
		authPort: authPort,
	}
}

func (c *Client) Authenticate(username, password string) (string, error) {
	// Mock authentication - in real implementation, this would call auth service
	if username == "" || password == "" {
		return "", errors.New("invalid credentials")
	}

	// Generate JWT
	token, err := c.generateJWT(username, "")
	if err != nil {
		return "", err
	}

	return token, nil
}

func (c *Client) Register(username, password, email string) error {
	// Mock registration - in real implementation, this would call auth service
	if username == "" || password == "" || email == "" {
		return errors.New("missing required fields")
	}

	// In real implementation, hash password and store user
	fmt.Printf("User registered: %s, %s\n", username, email)
	return nil
}

func (c *Client) ValidateToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte("your-secret-key"), nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}

	return nil, errors.New("invalid token")
}

func (c *Client) generateJWT(username, email string) (string, error) {
	expirationTime := time.Now().Add(24 * time.Hour)
	claims := &Claims{
		Username: username,
		Email:    email,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "ai-assistant-service",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte("your-secret-key"))
}

// Mock Auth Service (would be a separate microservice)
type AuthService struct {
	users map[string]User
}

type User struct {
	ID       string
	Username string
	Password string
	Email    string
}

func NewAuthService() *AuthService {
	return &AuthService{
		users: make(map[string]User),
	}
}

func (s *AuthService) Register(username, password, email string) error {
	if _, exists := s.users[username]; exists {
		return errors.New("user already exists")
	}

	user := User{
		ID:       generateUserID(),
		Username: username,
		Password: password, // In production, hash this
		Email:    email,
	}

	s.users[username] = user
	return nil
}

func (s *AuthService) Authenticate(username, password string) (*User, error) {
	user, exists := s.users[username]
	if !exists || user.Password != password {
		return nil, errors.New("invalid credentials")
	}

	return &user, nil
}

func (s *AuthService) GetUser(username string) (*User, error) {
	user, exists := s.users[username]
	if !exists {
		return nil, errors.New("user not found")
	}

	return &user, nil
}

func generateUserID() string {
	return fmt.Sprintf("user_%d", time.Now().UnixNano())
}

// Health check for auth service
func (c *Client) HealthCheck() bool {
	resp, err := http.Get("http://localhost:" + c.authPort + "/health")
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}