package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

var jwtSecretKey = []byte("screen-time-secret-change-in-prod-1234567890")

type UserClaims struct {
	UserID      string `json:"user_id"`
	FamilyID    string `json:"family_id"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"` // "parent"
	jwt.RegisteredClaims
}

type DeviceClaims struct {
	DeviceID string `json:"device_id"`
	ChildID  string `json:"child_id"`
	Role     string `json:"role"` // "device"
	jwt.RegisteredClaims
}

type PushActionClaims struct {
	FamilyID  string `json:"family_id"`
	RequestID string `json:"request_id"`
	Role      string `json:"role"` // "push_action"
	jwt.RegisteredClaims
}

func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), 10)
	return string(bytes), err
}

func CheckPasswordHash(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

func GenerateUserToken(userID, familyID, email, displayName string) (string, error) {
	claims := UserClaims{
		UserID:      userID,
		FamilyID:    familyID,
		Email:       email,
		DisplayName: displayName,
		Role:        "parent",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(30 * 24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecretKey)
}

func GenerateDeviceToken(deviceID, childID string) (string, error) {
	claims := DeviceClaims{
		DeviceID: deviceID,
		ChildID:  childID,
		Role:     "device",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(365 * 24 * time.Hour)), // 1 year
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecretKey)
}

func ValidateUserToken(tokenStr string) (*UserClaims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &UserClaims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return jwtSecretKey, nil
	})
	if err != nil {
		return nil, err
	}
	if claims, ok := token.Claims.(*UserClaims); ok && token.Valid {
		if claims.Role != "parent" {
			return nil, errors.New("invalid role for parent user")
		}
		return claims, nil
	}
	return nil, errors.New("invalid token")
}

func ValidateDeviceToken(tokenStr string) (*DeviceClaims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &DeviceClaims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return jwtSecretKey, nil
	})
	if err != nil {
		return nil, err
	}
	if claims, ok := token.Claims.(*DeviceClaims); ok && token.Valid {
		if claims.Role != "device" {
			return nil, errors.New("invalid role for device")
		}
		return claims, nil
	}
	return nil, errors.New("invalid token")
}

func GeneratePushActionToken(familyID, requestID string) (string, error) {
	claims := PushActionClaims{
		FamilyID:  familyID,
		RequestID: requestID,
		Role:      "push_action",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecretKey)
}

func ValidatePushActionToken(tokenStr string) (*PushActionClaims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &PushActionClaims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return jwtSecretKey, nil
	})
	if err != nil {
		return nil, err
	}
	if claims, ok := token.Claims.(*PushActionClaims); ok && token.Valid {
		if claims.Role != "push_action" {
			return nil, errors.New("invalid role for push action")
		}
		return claims, nil
	}
	return nil, errors.New("invalid token")
}

func GenerateSecureCode(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	// Convert to uppercase alphanumeric string (avoid confusing characters like 0, O, 1, I)
	const chars = "23456789ABCDEFGHJKLMNPQRSTUVWXYZ"
	result := make([]byte, length)
	for i, b := range bytes {
		result[i] = chars[int(b)%len(chars)]
	}
	return string(result), nil
}

func GenerateUUID() string {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

