// Package modem is the domain facade. The HTTP layer talks only to Modem;
// AT details are confined to facade.go and its sibling files.
package modem

import (
	"context"
	"time"
)

type ModemStatus struct {
	Model     string
	IMEI      string
	Firmware  string
	Epoch     int64
	SIM       SIMStatus
	Network   NetworkStatus
	BatteryV  float64
	UptimeSec int64
	UpdatedAt time.Time
}

type SIMStatus struct {
	State    string // ready | pin_required | absent
	ICCID    string
	IMSI     string
	Operator string
}

type NetworkStatus struct {
	Tech       string // LTE | UMTS | GSM | none
	Band       string
	RSRPdBm    int
	RSRQdB     int
	CSQ        int
	Registered bool
}

type SendOpts struct {
	IdemKey        string
	DeliveryReport bool
}

type Outbound struct {
	ID          string
	To          string
	Body        string
	State       string
	Parts       []OutboundPart
	Encoding    string
	ErrorCode   string
	ErrorDetail string
	CreatedAt   time.Time
}

type OutboundPart struct{ MR int }

type Inbound struct {
	ID         string
	From       string
	Body       string
	Encoding   string
	Parts      int
	Incomplete bool
	SMSCTime   time.Time
	ReceivedAt time.Time
}

type Call struct {
	ID         string
	Direction  string
	RemoteAddr string
	State      string
	EndReason  string
	StartedAt  time.Time
	AnsweredAt time.Time
	EndedAt    time.Time
	DurationMS int
}

// CallForwardingRule is the network-provisioned voice forwarding state for a
// single condition. TimeoutSeconds is only meaningful for no_reply.
type CallForwardingRule struct {
	Reason         string
	Enabled        bool
	Number         string
	TimeoutSeconds int
	Available      bool
	Error          string
}

type IncomingCall struct{ ID, From string }

type SMSArrived struct{ ID, From, Body string }

type LifecycleEvent struct {
	Kind   string // modem.reset | transport.flap
	Detail string
}

type Modem interface {
	Status(ctx context.Context, refresh bool) (ModemStatus, error)

	SendSMS(ctx context.Context, to, body string, opts SendOpts) (Outbound, error)
	GetOutbound(ctx context.Context, id string) (Outbound, error)
	GetInbound(ctx context.Context, id string) (Inbound, error)
	ListInbound(ctx context.Context, since string, limit int) ([]Inbound, error)
	ListOutbound(ctx context.Context, since string, limit int) ([]Outbound, error)
	DeleteSMS(ctx context.Context, id string) error

	Dial(ctx context.Context, to string, idem string) (Call, error)
	Hangup(ctx context.Context, callID string) error
	Answer(ctx context.Context, callID string) error
	Reject(ctx context.Context, callID string) error
	Hold(ctx context.Context, callID string) error
	Resume(ctx context.Context, callID string) error
	MergeCalls(ctx context.Context, callID string) error
	SendDTMF(ctx context.Context, callID, digits string, durMS int) error
	StartCallAudio(ctx context.Context, callID string) error
	StopCallAudio(ctx context.Context, callID string) error
	GetCall(ctx context.Context, id string) (Call, error)
	ListCalls(ctx context.Context, since string, limit int) ([]Call, error)

	GetCallForwarding(ctx context.Context) ([]CallForwardingRule, error)
	SetCallForwarding(ctx context.Context, rule CallForwardingRule) (CallForwardingRule, error)

	OnIncomingCall() <-chan IncomingCall
	OnInboundSMS() <-chan SMSArrived
	OnLifecycle() <-chan LifecycleEvent
}
