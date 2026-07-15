package modem

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"sim7600d/internal/simmodem"
	"sim7600d/internal/store"
)

func TestInbound_CMTIIngest(t *testing.T) {
	f, sim := newTestFacade(t)
	for _, c := range []string{"ATE0", "AT+CMEE=2", "AT+CMGF=0", "AT+CNMI=2,1,0,1,0", "AT+CLIP=1", "AT+CRC=1", `AT+CSCS="UCS2"`} {
		sim.OnExact(c, "OK")
	}
	require.NoError(t, f.Boot(context.Background()))

	// AT+CMGR=5 returns a synthetic single-part DELIVER PDU.
	// The fixture hex decodes to: From="+15551234567", Body="Hello", Encoding="gsm7"
	// This is the same hex used in internal/sms/pdu_test.go deliverFixtureHex().
	sim.OnExact("AT+CMGR=5", "+CMGR: 0,,17", deliverFixturePDU(), "OK")
	sim.OnExact("AT+CMGD=5", "OK")

	// emit URC
	sim.EmitURC(`+CMTI: "ME",5`)

	select {
	case got := <-f.OnInboundSMS():
		require.Equal(t, "+15551234567", got.From)
		require.Equal(t, "Hello", got.Body)
		require.NotEmpty(t, got.ID)
		events, err := f.st.ListEvents(context.Background(), store.EventFilter{Kind: "sms.arrived", Limit: 1})
		require.NoError(t, err)
		require.Len(t, events, 1)
		var detail map[string]string
		require.NoError(t, json.Unmarshal([]byte(events[0].Detail), &detail))
		require.Equal(t, "+15551234567", detail["from"])
		require.Equal(t, "Hello", detail["body"])
	case <-time.After(2 * time.Second):
		t.Fatal("expected inbound SMS event")
	}
}

// deliverFixturePDU returns the same DELIVER PDU hex used in internal/sms/pdu_test.go.
// Decodes to: From="+15551234567", Body="Hello", Encoding="gsm7"
func deliverFixturePDU() string {
	// Complete modem PDU: six-byte SMSC prefix followed by SMS-DELIVER TPDU.
	return "06912143658709" + "000B915155214365F700004210510103000005C8329BFD06"
}

// suppress unused import warning if simmodem is only used via newTestFacade
var _ = (*simmodem.Sim)(nil)
