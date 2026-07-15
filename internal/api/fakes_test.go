package api

import (
	"context"
	"database/sql"

	"sim7600d/internal/modem"
)

type fakeModem struct {
	status        modem.ModemStatus
	sendErr       error
	sendOut       modem.Outbound
	dial          modem.Call
	dialErr       error
	listInbound   []modem.Inbound
	listOutbound  []modem.Outbound
	listCalls     []modem.Call
	getCall       modem.Call
	getCallErr    error
	hangupErr     error
	answerErr     error
	rejectErr     error
	holdErr       error
	resumeErr     error
	mergeErr      error
	mergedID      string
	dtmfErr       error
	forwarding    []modem.CallForwardingRule
	forwardingErr error
	forwardingSet modem.CallForwardingRule

	incoming chan modem.IncomingCall
	inbound  chan modem.SMSArrived
	life     chan modem.LifecycleEvent
}

func (f *fakeModem) Status(ctx context.Context, refresh bool) (modem.ModemStatus, error) {
	return f.status, nil
}
func (f *fakeModem) SendSMS(ctx context.Context, to, body string, opts modem.SendOpts) (modem.Outbound, error) {
	return f.sendOut, f.sendErr
}
func (f *fakeModem) GetOutbound(ctx context.Context, id string) (modem.Outbound, error) {
	return f.sendOut, nil
}
func (f *fakeModem) GetInbound(ctx context.Context, id string) (modem.Inbound, error) {
	for _, it := range f.listInbound {
		if it.ID == id {
			return it, nil
		}
	}
	return modem.Inbound{}, sql.ErrNoRows
}
func (f *fakeModem) DeleteSMS(ctx context.Context, id string) error { return nil }
func (f *fakeModem) ListInbound(ctx context.Context, since string, limit int) ([]modem.Inbound, error) {
	return f.listInbound, nil
}
func (f *fakeModem) ListOutbound(ctx context.Context, since string, limit int) ([]modem.Outbound, error) {
	return f.listOutbound, nil
}
func (f *fakeModem) Dial(ctx context.Context, to, idem string) (modem.Call, error) {
	return f.dial, f.dialErr
}
func (f *fakeModem) Hangup(ctx context.Context, id string) error { return f.hangupErr }
func (f *fakeModem) Answer(ctx context.Context, id string) error { return f.answerErr }
func (f *fakeModem) Reject(ctx context.Context, id string) error { return f.rejectErr }
func (f *fakeModem) Hold(ctx context.Context, id string) error   { return f.holdErr }
func (f *fakeModem) Resume(ctx context.Context, id string) error { return f.resumeErr }
func (f *fakeModem) MergeCalls(ctx context.Context, id string) error {
	f.mergedID = id
	return f.mergeErr
}
func (f *fakeModem) SendDTMF(ctx context.Context, id, digits string, dur int) error {
	return f.dtmfErr
}
func (f *fakeModem) StartCallAudio(ctx context.Context, id string) error { return nil }
func (f *fakeModem) StopCallAudio(ctx context.Context, id string) error  { return nil }
func (f *fakeModem) GetCall(ctx context.Context, id string) (modem.Call, error) {
	return f.getCall, f.getCallErr
}
func (f *fakeModem) ListCalls(ctx context.Context, since string, limit int) ([]modem.Call, error) {
	return f.listCalls, nil
}
func (f *fakeModem) GetCallForwarding(ctx context.Context) ([]modem.CallForwardingRule, error) {
	return f.forwarding, f.forwardingErr
}
func (f *fakeModem) SetCallForwarding(ctx context.Context, rule modem.CallForwardingRule) (modem.CallForwardingRule, error) {
	f.forwardingSet = rule
	if f.forwardingErr != nil {
		return modem.CallForwardingRule{}, f.forwardingErr
	}
	return rule, nil
}
func (f *fakeModem) OnIncomingCall() <-chan modem.IncomingCall {
	if f.incoming == nil {
		f.incoming = make(chan modem.IncomingCall)
	}
	return f.incoming
}
func (f *fakeModem) OnInboundSMS() <-chan modem.SMSArrived {
	if f.inbound == nil {
		f.inbound = make(chan modem.SMSArrived)
	}
	return f.inbound
}
func (f *fakeModem) OnLifecycle() <-chan modem.LifecycleEvent {
	if f.life == nil {
		f.life = make(chan modem.LifecycleEvent)
	}
	return f.life
}

// Compile-time interface check.
var _ modem.Modem = (*fakeModem)(nil)
