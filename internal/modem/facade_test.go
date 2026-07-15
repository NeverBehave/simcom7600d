package modem

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"sim7600d/internal/atexec"
	"sim7600d/internal/simmodem"
	"sim7600d/internal/store"
	"sim7600d/internal/urc"
)

func newTestFacade(t *testing.T) (*Facade, *simmodem.Sim) {
	t.Helper()
	sim, peer := simmodem.New(t)
	bus := urc.NewBus()
	ex := atexec.New(peer, bus)
	st, err := store.Open(":memory:")
	require.NoError(t, err)
	f, err := New(ex, bus, st)
	require.NoError(t, err)
	t.Cleanup(func() { f.Close(); ex.Close(); bus.Close(); st.Close() })
	return f, sim
}

func TestFacade_StatusComposite(t *testing.T) {
	f, sim := newTestFacade(t)

	sim.OnExact("ATE0", "OK")
	sim.OnExact("AT+CMEE=2", "OK")
	sim.OnExact("AT+CMGF=0", "OK")
	sim.OnExact("AT+CNMI=2,1,0,1,0", "OK")
	sim.OnExact("AT+CLIP=1", "OK")
	sim.OnExact("AT+CRC=1", "OK")
	sim.OnExact(`AT+CSCS="UCS2"`, "OK")

	sim.OnExact("ATI", "Manufacturer: SIMCOM INCORPORATED", "Model: SIMCOM_SIM7600G-H", "Revision: SIM7600G_V2.0.2", "IMEI: 000000000000000", "+GCAP: +CGSM", "OK")
	sim.OnExact("AT+CSQ", "+CSQ: 20,99", "OK")
	sim.OnExact("AT+CPIN?", `+CPIN: READY`, "OK")
	sim.OnExact("AT+COPS?", `+COPS: 0,0,"Helium",7`, "OK")
	sim.OnExact("AT+CREG?", `+CREG: 0,1`, "OK")
	sim.OnExact("AT+CBC", `+CBC: 3.95V`, "OK")
	sim.OnExact("AT+CCID", `+CCID: 8901000000000000000`, "OK")
	sim.OnExact("AT+CIMI", "001010123456789", "OK")
	sim.OnExact("AT+CPSI?", `+CPSI: LTE,Online,310-260,0x3A40,21044744,126,EUTRAN-BAND2,650,3,3,-166,-1079,-740,9`, "OK")

	require.NoError(t, f.Boot(context.Background()))

	got, err := f.Status(context.Background(), true)
	require.NoError(t, err)
	require.Equal(t, "SIMCOM_SIM7600G-H", got.Model)
	require.Equal(t, "000000000000000", got.IMEI)
	require.Equal(t, "ready", got.SIM.State)
	require.Equal(t, "Helium", got.SIM.Operator)
	require.Equal(t, "LTE", got.Network.Tech)
	require.Equal(t, "EUTRAN-BAND2", got.Network.Band)
	require.True(t, got.Network.Registered)
	require.Equal(t, 20, got.Network.CSQ)
	require.Equal(t, -107, got.Network.RSRPdBm)
	require.Equal(t, -16, got.Network.RSRQdB)
	require.InDelta(t, 3.95, got.BatteryV, 0.01)
	_ = time.Time{}
}
