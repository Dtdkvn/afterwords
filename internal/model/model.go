// Package model contains the persistence and API domain types.
package model

import "time"

const (
	StateActive    = "ACTIVE"
	StateOverdue   = "OVERDUE"
	StateReleasing = "RELEASING"
	StateReleased  = "RELEASED"
	StatePaused    = "PAUSED"
)

// ReminderSchedule controls reminder offsets in virtual days.
type ReminderSchedule struct {
	BeforeDays  []int `json:"beforeDays"`
	OverdueDays []int `json:"overdueDays"`
}

// DefaultReminderSchedule is calm but increasingly visible around a deadline.
func DefaultReminderSchedule() ReminderSchedule {
	return ReminderSchedule{BeforeDays: []int{3, 1}, OverdueDays: []int{0, 1, 2}}
}

type User struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"createdAt"`
}

type Session struct {
	ID        string
	UserID    string
	CSRFToken string
	ExpiresAt time.Time
}

type Switch struct {
	ID                 string           `json:"id"`
	UserID             string           `json:"-"`
	Name               string           `json:"name"`
	IntervalDays       int              `json:"intervalDays"`
	GraceDays          int              `json:"graceDays"`
	State              string           `json:"state"`
	NextDeadline       time.Time        `json:"nextDeadline"`
	LastCheckin        time.Time        `json:"lastCheckin"`
	Reminders          ReminderSchedule `json:"reminders"`
	ReleaseToken       string           `json:"-"`
	RecoveryThreshold  int              `json:"recoveryThreshold"`
	RecoveryShareCount int              `json:"recoveryShareCount"`
	FireDrillAt        *time.Time       `json:"fireDrillAt,omitempty"`
	CreatedAt          time.Time        `json:"createdAt"`
	UpdatedAt          time.Time        `json:"updatedAt"`
}

type Keyslot struct {
	ID        string         `json:"id"`
	SwitchID  string         `json:"switchId"`
	WrappedDK string         `json:"wrappedDk"`
	IV        string         `json:"iv"`
	Salt      string         `json:"salt"`
	KDFParams map[string]int `json:"kdfParams"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
}

type VaultItem struct {
	ID         string    `json:"id"`
	SwitchID   string    `json:"switchId,omitempty"`
	IV         string    `json:"iv"`
	Ciphertext string    `json:"ciphertext"`
	AAD        string    `json:"aad"`
	Size       int64     `json:"size"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

type Recipient struct {
	ID        string    `json:"id"`
	SwitchID  string    `json:"-"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"createdAt"`
}

type Trustee struct {
	ID               string     `json:"id"`
	SwitchID         string     `json:"-"`
	Name             string     `json:"name"`
	Email            string     `json:"email"`
	ShareIndex       int        `json:"shareIndex"`
	ShareFingerprint string     `json:"shareFingerprint"`
	KitDeliveredAt   *time.Time `json:"kitDeliveredAt,omitempty"`
	CreatedAt        time.Time  `json:"createdAt"`
}

type AuditEntry struct {
	ID        string         `json:"id"`
	SwitchID  string         `json:"switchId"`
	Event     string         `json:"event"`
	Meta      map[string]any `json:"meta"`
	CreatedAt time.Time      `json:"createdAt"`
}

type OutboxMessage struct {
	ID          string
	SwitchID    string
	Kind        string
	ToEmail     string
	Subject     string
	BodyHTML    string
	BodyText    string
	ScheduledAt time.Time
	SentAt      *time.Time
	Attempts    int
	LastError   string
}
