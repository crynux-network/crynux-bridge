package tools

import (
	"context"
	"crynux_bridge/api/ratelimit"
	"crynux_bridge/api/v1/response"
	"crynux_bridge/models"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"slices"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

func GenerateAPIKey(ctx context.Context, db *gorm.DB, clientID string) (string, int64, error) {
	randKey := make([]byte, 32)
	_, err := rand.Read(randKey)
	if err != nil {
		return "", 0, err
	}
	apiKeyStr := base64.URLEncoding.EncodeToString(randKey)
	keyPrefix := apiKeyStr[:8]
	hashKey, err := bcrypt.GenerateFromPassword(randKey, bcrypt.DefaultCost)
	if err != nil {
		return "", 0, err
	}
	hashKeyStr := base64.URLEncoding.EncodeToString(hashKey)

	now := time.Now()
	expiresAt := now.Add(time.Hour * 24 * 365) // 1 year expiration
	// Generate a new API key
	var apiKey *models.ClientAPIKey
	apiKey, err = models.GetAPIKeyByClientID(ctx, db, clientID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		apiKey = &models.ClientAPIKey{
			ClientID:   clientID,
			KeyPrefix:  keyPrefix,
			KeyHash:    hashKeyStr,
			LastUsedAt: now,
			ExpiresAt:  expiresAt, // 1 year expiration
			UsedCount:  0,
			UseLimit:   20,
		}
		if err := apiKey.Save(ctx, db); err != nil {
			return "", 0, err
		}
	} else if err != nil {
		return "", 0, err
	} else {
		apiKey.KeyPrefix = keyPrefix
		apiKey.KeyHash = hashKeyStr
		apiKey.LastUsedAt = now
		apiKey.ExpiresAt = expiresAt
		apiKey.UsedCount = 0
		apiKey.UseLimit = 20
		if err := apiKey.Save(ctx, db); err != nil {
			return "", 0, err
		}
	}

	return apiKeyStr, expiresAt.Unix(), nil
}

var ErrAPIKeyInvalid = errors.New("API key is invalid")
var ErrAPIKeyExpired = errors.New("API key is expired")

func ValidateAPIKey(ctx context.Context, db *gorm.DB, apiKeyStr string) (*models.ClientAPIKey, error) {
	rawKey, err := base64.URLEncoding.DecodeString(apiKeyStr)
	if err != nil {
		return nil, ErrAPIKeyInvalid
	}
	keyPrefix := apiKeyStr[:8]
	apiKey, err := models.GetAPIKeyByKeyPrefix(ctx, db, keyPrefix)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAPIKeyInvalid
		}
		return nil, err
	}
	hashKey, _ := base64.URLEncoding.DecodeString(apiKey.KeyHash)
	if apiKey.ExpiresAt.Before(time.Now()) {
		return nil, ErrAPIKeyExpired
	}
	err = bcrypt.CompareHashAndPassword(hashKey, rawKey)
	if err != nil {
		return nil, ErrAPIKeyInvalid
	}
	return apiKey, nil
}

func DeleteAPIKey(ctx context.Context, db *gorm.DB, apiKey *models.ClientAPIKey) error {
	return apiKey.Delete(ctx, db)
}

func AddAPIKeyRole(ctx context.Context, db *gorm.DB, apiKey *models.ClientAPIKey, role models.Role) error {
	if slices.Contains(apiKey.Roles, role) {
		return nil
	}
	newAPIKey := &models.ClientAPIKey{
		Roles: apiKey.Roles,
	}
	newAPIKey.Roles = append(newAPIKey.Roles, role)

	return apiKey.Update(ctx, db, newAPIKey)
}

func ChangeUseLimit(ctx context.Context, db *gorm.DB, apiKey *models.ClientAPIKey, useLimit int64) error {
	return apiKey.Update(ctx, db, &models.ClientAPIKey{
		UseLimit: useLimit,
	})
}

func ChangeRateLimit(ctx context.Context, db *gorm.DB, apiKey *models.ClientAPIKey, rateLimit int64) error {
	if err := apiKey.Update(ctx, db, &models.ClientAPIKey{
		RateLimit: rateLimit,
	}); err != nil {
		return err
	}

	return ratelimit.APIRateLimiter.UpdateRateLimit(ctx, apiKey.ClientID, rateLimit, time.Minute)
}

// validate api key
func ValidateAuthorization(ctx context.Context, db *gorm.DB, authorization string) (*models.ClientAPIKey, error) {
	if !strings.HasPrefix(authorization, "Bearer ") {
		return nil, response.NewValidationErrorResponse("Authorization", "Authorization header must start with 'Bearer '")
	}
	apiKeyStr := authorization[7:]
	apiKey, err := ValidateAPIKey(ctx, db, apiKeyStr)
	if err != nil {
		if errors.Is(err, ErrAPIKeyExpired) {
			return nil, response.NewValidationErrorResponse("Authorization", "API key has expired")
		}
		if errors.Is(err, ErrAPIKeyInvalid) {
			return nil, response.NewValidationErrorResponse("Authorization", "invalid API key")
		}
		return nil, response.NewExceptionResponse(err)
	}
	if !slices.Contains(apiKey.Roles, models.RoleAdmin) && !slices.Contains(apiKey.Roles, models.RoleChat) {
		return nil, response.NewValidationErrorResponse("Authorization", "API key does not have required role: admin or chat")
	}
	if apiKey.UseLimit > 0 && apiKey.UsedCount >= apiKey.UseLimit {
		return nil, response.NewValidationErrorResponse("Authorization", "API key quota exceeded")
	}

	return apiKey, nil
}
