package voicemail

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"sim7600d/internal/modem"
	"sim7600d/internal/store"
)

func TestParseMailboxUpdateValidatesTMobileIMAPS(t *testing.T) {
	got, err := ParseMailboxUpdate("MBOXUPDATE?;m=1;name=15551234567;server=e7.vvm.mstore.msg.t-mobile.com;port=993;pw=temporary")
	require.NoError(t, err)
	require.Equal(t, "e7.vvm.mstore.msg.t-mobile.com", got.Server)
	require.Equal(t, "15551234567", got.Username)

	_, err = ParseMailboxUpdate("MBOXUPDATE?;name=user;server=127.0.0.1;port=993;pw=x")
	require.ErrorContains(t, err, "unsupported")
	_, err = ParseMailboxUpdate("MBOXUPDATE?;name=user;server=e7.vvm.mstore.msg.t-mobile.com;port=143;pw=x")
	require.ErrorContains(t, err, "993")
}

func TestServiceSyncStoresFetchedVoicemail(t *testing.T) {
	st, err := store.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = st.Close() })
	m := &fakeInbox{messages: []modem.Inbound{{
		ID: "sms-1", Body: "MBOXUPDATE?;name=user;server=e7.vvm.mstore.msg.t-mobile.com;port=993;pw=secret", ReceivedAt: time.Now().UTC(),
	}}}
	fetcher := &fakeFetcher{messages: []RemoteMessage{{
		SourceID: "uid-1", From: "+15551234567", ReceivedAt: time.Now().UTC(),
		DurationMS: 9000, ContentType: "audio/amr", Audio: []byte("voice"),
	}}}
	service := NewWithFetcher(st, m, fetcher)
	service.transcoder = fakeTranscoder{output: []byte("RIFFvoice")}
	result, err := service.Sync(context.Background())
	require.NoError(t, err)
	require.Equal(t, SyncResult{Fetched: 1, Added: 1}, result)
	items, err := st.ListVoicemails(context.Background(), store.VoicemailFilter{})
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, "sms-1", items[0].SourceSMSID)
	require.Equal(t, "user", fetcher.credentials.Username)
	stored, err := st.GetVoicemail(context.Background(), items[0].ID, true)
	require.NoError(t, err)
	require.Equal(t, "audio/wav", stored.ContentType)
	require.Equal(t, []byte("RIFFvoice"), stored.Audio)
}

type fakeTranscoder struct{ output []byte }

func (f fakeTranscoder) ToWAV(context.Context, []byte) ([]byte, error) { return f.output, nil }

type fakeFetcher struct {
	credentials Credentials
	messages    []RemoteMessage
}

func (f *fakeFetcher) Fetch(_ context.Context, credentials Credentials) ([]RemoteMessage, error) {
	f.credentials = credentials
	return f.messages, nil
}

type fakeInbox struct{ messages []modem.Inbound }

func (f *fakeInbox) ListInbound(context.Context, string, int) ([]modem.Inbound, error) {
	return f.messages, nil
}
func (f *fakeInbox) Status(context.Context, bool) (modem.ModemStatus, error) {
	return modem.ModemStatus{}, nil
}
func (f *fakeInbox) SendSMS(context.Context, string, string, modem.SendOpts) (modem.Outbound, error) {
	return modem.Outbound{}, nil
}
func (f *fakeInbox) GetOutbound(context.Context, string) (modem.Outbound, error) {
	return modem.Outbound{}, sql.ErrNoRows
}
func (f *fakeInbox) GetInbound(context.Context, string) (modem.Inbound, error) {
	return modem.Inbound{}, sql.ErrNoRows
}
func (f *fakeInbox) ListOutbound(context.Context, string, int) ([]modem.Outbound, error) {
	return nil, nil
}
func (f *fakeInbox) DeleteSMS(context.Context, string) error { return nil }
func (f *fakeInbox) Dial(context.Context, string, string) (modem.Call, error) {
	return modem.Call{}, nil
}
func (f *fakeInbox) Hangup(context.Context, string) error                { return nil }
func (f *fakeInbox) Answer(context.Context, string) error                { return nil }
func (f *fakeInbox) Reject(context.Context, string) error                { return nil }
func (f *fakeInbox) Hold(context.Context, string) error                  { return nil }
func (f *fakeInbox) Resume(context.Context, string) error                { return nil }
func (f *fakeInbox) MergeCalls(context.Context, string) error            { return nil }
func (f *fakeInbox) SendDTMF(context.Context, string, string, int) error { return nil }
func (f *fakeInbox) StartCallAudio(context.Context, string) error        { return nil }
func (f *fakeInbox) StopCallAudio(context.Context, string) error         { return nil }
func (f *fakeInbox) GetCall(context.Context, string) (modem.Call, error) {
	return modem.Call{}, sql.ErrNoRows
}
func (f *fakeInbox) ListCalls(context.Context, string, int) ([]modem.Call, error) { return nil, nil }
func (f *fakeInbox) GetCallForwarding(context.Context) ([]modem.CallForwardingRule, error) {
	return nil, nil
}
func (f *fakeInbox) SetCallForwarding(context.Context, modem.CallForwardingRule) (modem.CallForwardingRule, error) {
	return modem.CallForwardingRule{}, nil
}
func (f *fakeInbox) OnIncomingCall() <-chan modem.IncomingCall { return make(chan modem.IncomingCall) }
func (f *fakeInbox) OnInboundSMS() <-chan modem.SMSArrived     { return make(chan modem.SMSArrived) }
func (f *fakeInbox) OnLifecycle() <-chan modem.LifecycleEvent  { return make(chan modem.LifecycleEvent) }
