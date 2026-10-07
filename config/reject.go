package config

import "fmt"

// RejectReason 是可区分的配置拒绝原因。
type RejectReason string

const (
	ReasonMissingKey   RejectReason = "missing_key"
	ReasonTypeMismatch RejectReason = "type_mismatch"
	ReasonOutOfRange   RejectReason = "out_of_range"
	ReasonUnknownKey   RejectReason = "unknown_key"
)

// Rejection 描述一次针对具体键的拒绝。
type Rejection struct {
	Source string
	Key    string
	Reason RejectReason
	Detail string
}

func (r Rejection) Error() string {
	return fmt.Sprintf("config rejected: source=%q key=%q reason=%s: %s", r.Source, r.Key, r.Reason, r.Detail)
}
