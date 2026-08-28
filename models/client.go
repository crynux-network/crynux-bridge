package models

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

type Client struct {
	RootModel
	ClientId string `json:"client_id" gorm:"uniqueIndex"`
}

type ClientTaskStatus string

const (
	ClientTaskStatusRunning ClientTaskStatus = "running"
	ClientTaskStatusSuccess ClientTaskStatus = "success"
	ClientTaskStatusFailed  ClientTaskStatus = "failed"
)

type ClientTask struct {
	RootModel
	ClientID           uint             `json:"client_id" gorm:"index:idx_client_task_pending_submission,priority:1"`
	Status             ClientTaskStatus `json:"status"`
	FailedCount        int              `json:"failed_count"`
	Submission         string           `json:"-" gorm:"type:longtext"`
	SubmissionTaskType ChainTaskType    `json:"-" gorm:"index:idx_client_task_pending_submission,priority:3"`
	SubmissionModelID  string           `json:"-" gorm:"index:idx_client_task_pending_submission,priority:4"`
	EffectiveRepeatNum int              `json:"-" gorm:"not null;default:1"`
	RepeatExpanded     bool             `json:"-" gorm:"index:idx_client_task_expansion,priority:1;index:idx_client_task_pending_submission,priority:2"`
	NextActionAt       time.Time        `json:"-" gorm:"index:idx_client_task_expansion,priority:2"`
	ExpansionError     string           `json:"-" gorm:"type:text"`
	Client             Client           `json:"-"`
	InferenceTasks     []InferenceTask  `json:"-"`
}

func (task *ClientTask) BeforeCreate(*gorm.DB) error {
	task.Status = ClientTaskStatusRunning
	if task.NextActionAt.IsZero() {
		task.NextActionAt = time.Now()
	}
	return nil
}

func GetClientTaskByID(ctx context.Context, db *gorm.DB, clientTaskID uint) (*ClientTask, error) {
	dbCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	clientTask := ClientTask{
		RootModel: RootModel{
			ID: clientTaskID,
		},
	}
	err := db.WithContext(dbCtx).Model(clientTask).Where(&clientTask).First(&clientTask).Error
	if err != nil {
		return nil, err
	}
	return &clientTask, nil
}

func (task *ClientTask) Update(ctx context.Context, db *gorm.DB, newTask *ClientTask) error {
	if task.ID == 0 {
		return errors.New("ClientTask.ID cannot be 0 when update")
	}
	dbCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return db.WithContext(dbCtx).Model(task).Updates(newTask).Error
}

type Role string

const (
	RoleAdmin Role = "admin"
	RoleChat  Role = "chat"
	RoleImage Role = "image"
)

type Roles []Role

func (roles *Roles) Scan(val interface{}) error {
	var arrString string
	switch v := val.(type) {
	case string:
		arrString = v
	case []byte:
		arrString = string(v)
	case nil:
		return nil
	default:
		return errors.New(fmt.Sprint("Unable to parse value to Roles: ", val))
	}
	arr := strings.Split(arrString, ",")
	*roles = make([]Role, 0)
	for _, v := range arr {
		if len(v) > 0 {
			*roles = append(*roles, Role(v))
		}
	}
	return nil
}

func (roles Roles) Value() (driver.Value, error) {
	arr := make([]string, len(roles))
	for i, v := range roles {
		arr[i] = string(v)
	}
	res := strings.Join(arr, ",")
	return res, nil
}

type ClientAPIKey struct {
	RootModel
	ClientID   string    `json:"client_id"`
	KeyPrefix  string    `json:"key_prefix" gorm:"index"`
	KeyHash    string    `json:"key_hash"`
	ExpiresAt  time.Time `json:"expires_at"`
	LastUsedAt time.Time `json:"last_used_at"`
	Roles      Roles     `json:"roles"`
	Client     Client    `json:"-" gorm:"foreignKey:ClientID;references:ClientId"`
	UsedCount  int64     `json:"used_count" gorm:"default:0"`
	UseLimit   int64     `json:"use_limit" gorm:"default:20"`
	RateLimit  int64     `json:"rate_limit" gorm:"default:1"`
}

var ErrAPIKeyQuotaExceeded = errors.New("API key quota exceeded")

func (key *ClientAPIKey) Save(ctx context.Context, db *gorm.DB) error {
	dbCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	return db.WithContext(dbCtx).Save(key).Error
}

func (key *ClientAPIKey) Update(ctx context.Context, db *gorm.DB, newKey *ClientAPIKey) error {
	if key.ID == 0 {
		return errors.New("ClientAPIKey.ID cannot be 0 when update")
	}
	dbCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	return db.WithContext(dbCtx).Model(key).Updates(newKey).Error
}

func (key *ClientAPIKey) Use(ctx context.Context, db *gorm.DB) error {
	dbCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	updates := map[string]interface{}{
		"used_count":   gorm.Expr("used_count + ?", 1),
		"last_used_at": time.Now(),
	}
	return db.WithContext(dbCtx).Model(key).Updates(updates).Error
}

func (key *ClientAPIKey) UseWithinLimit(ctx context.Context, db *gorm.DB) error {
	dbCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	result := db.WithContext(dbCtx).Model(&ClientAPIKey{}).
		Where("id = ?", key.ID).
		Where("use_limit <= 0 OR used_count < use_limit").
		Updates(map[string]interface{}{
			"used_count":   gorm.Expr("used_count + ?", 1),
			"last_used_at": time.Now(),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrAPIKeyQuotaExceeded
	}
	return nil
}

func (key *ClientAPIKey) Delete(ctx context.Context, db *gorm.DB) error {
	if key.ID == 0 {
		return errors.New("ClientAPIKey.ID cannot be 0 when update")
	}
	dbCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	return db.WithContext(dbCtx).Model(key).Delete(key).Error
}

func GetAPIKeyByClientID(ctx context.Context, db *gorm.DB, clientID string) (*ClientAPIKey, error) {
	apiKey := ClientAPIKey{ClientID: clientID}
	dbCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	err := db.WithContext(dbCtx).Model(apiKey).Where(&apiKey).First(&apiKey).Error
	if err != nil {
		return nil, err
	}
	return &apiKey, nil
}

func GetAPIKeyByKeyPrefix(ctx context.Context, db *gorm.DB, keyPrefix string) (*ClientAPIKey, error) {
	apiKey := ClientAPIKey{KeyPrefix: keyPrefix}
	dbCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	err := db.WithContext(dbCtx).Model(apiKey).Where(&apiKey).First(&apiKey).Error
	if err != nil {
		return nil, err
	}
	return &apiKey, nil
}
