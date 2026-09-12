package domain

import "time"

const (
	// 认证与安全
	ActionAuthLogin       = "AUTH_LOGIN"
	ActionAuthLoginFailed = "AUTH_LOGIN_FAILED"
	ActionAuthPassword    = "AUTH_PASSWORD_CHANGE"
	ActionAuth2FAEnable   = "AUTH_2FA_ENABLE"
	ActionAuth2FADisable  = "AUTH_2FA_DISABLE"

	// 用户管理
	ActionUserCreate       = "USER_CREATE"
	ActionUserUpdate       = "USER_UPDATE"
	ActionUserDelete       = "USER_DELETE"
	ActionUserResetTraffic = "USER_RESET_TRAFFIC"
	ActionUserResetToken   = "USER_RESET_TOKEN"
	ActionUserBatchRenew   = "USER_BATCH_RENEW"
	ActionUserBatchStatus  = "USER_BATCH_STATUS"

	// 入站与分流
	ActionInboundCreate = "INBOUND_CREATE"
	ActionInboundUpdate = "INBOUND_UPDATE"
	ActionInboundDelete = "INBOUND_DELETE"
	ActionRoutingSave   = "ROUTING_SAVE"
	ActionOutboundSave  = "OUTBOUND_SAVE"

	// 核心与配置
	ActionConfigApply    = "CONFIG_APPLY"
	ActionConfigRollback = "CONFIG_ROLLBACK"
	ActionCoreRestart    = "CORE_RESTART"

	// 运维与系统
	ActionLogClear      = "LOG_CLEAR"
	ActionAuditClear    = "AUDIT_LOG_CLEAR"
	ActionSettingUpdate = "SETTING_UPDATE"
)

type AuditLog struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	CreatedAt time.Time `gorm:"index" json:"created_at"`
	Operator  string    `gorm:"type:varchar(64);index" json:"operator"`
	IP        string    `gorm:"type:varchar(64)" json:"ip"`
	Action    string    `gorm:"type:varchar(64);index" json:"action"`
	Target    string    `gorm:"type:varchar(128)" json:"target"`
	Details   string    `gorm:"type:text" json:"details"`
	Status    string    `gorm:"type:varchar(32);default:'SUCCESS'" json:"status"`
}
