package modem

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"sim7600d/internal/atproto"
	"sim7600d/internal/simmodem"
	"sim7600d/internal/store"
)

func TestSendSMS_SinglePart(t *testing.T) {
	f, sim := newTestFacade(t)
	// boot configs
	for _, c := range []string{"ATE0", "AT+CMEE=2", "AT+CMGF=0", "AT+CNMI=2,1,0,1,0", "AT+CLIP=1", "AT+CRC=1", `AT+CSCS="UCS2"`} {
		sim.OnExact(c, "OK")
	}
	require.NoError(t, f.Boot(context.Background()))

	// AT+CMGS=<len> -> prompt -> +CMGS: 7 / OK
	sim.OnPrefix("AT+CMGS=", simmodem.PromptThen("+CMGS: 7", "OK"))

	out, err := f.SendSMS(context.Background(), "+15551234567", "hi", SendOpts{})
	require.NoError(t, err)
	require.Equal(t, "submitted", out.State)
	require.Len(t, out.Parts, 1)
	require.Equal(t, 7, out.Parts[0].MR)
	require.NotEmpty(t, out.ID)
}

func TestListOutbound_DoesNotDeadlockSingleConnectionStore(t *testing.T) {
	f, _ := newTestFacade(t)
	now := time.Now().UTC()
	require.NoError(t, f.st.InsertOutbound(context.Background(), store.Outbound{
		ID: "01TEST", ToAddr: "+15551234567", Body: "hi", Encoding: "gsm7",
		Parts: 1, State: "submitted", CreatedAt: now, UpdatedAt: now,
	}))

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	items, err := f.ListOutbound(ctx, "", 10)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, "01TEST", items[0].ID)
}

func TestSendSMS_CMSError(t *testing.T) {
	f, sim := newTestFacade(t)
	for _, c := range []string{"ATE0", "AT+CMEE=2", "AT+CMGF=0", "AT+CNMI=2,1,0,1,0", "AT+CLIP=1", "AT+CRC=1", `AT+CSCS="UCS2"`} {
		sim.OnExact(c, "OK")
	}
	require.NoError(t, f.Boot(context.Background()))
	// Sim writes ERROR after prompt.
	sim.OnPrefix("AT+CMGS=", func(_ string, w io.Writer, more func() ([]byte, error)) {
		_, _ = w.Write([]byte("\r\n> "))
		_, _ = more()
		_, _ = w.Write([]byte("\r\n+CMS ERROR: 500\r\n"))
	})

	_, err := f.SendSMS(context.Background(), "+15551234567", "hi", SendOpts{})
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "500") || strings.Contains(err.Error(), atproto.CMSMeaning(500)))
}
