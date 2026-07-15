package api

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"sim7600d/internal/modem"
)

type statusGetInput struct {
	Authorization string `header:"Authorization" required:"true" doc:"Bearer token"`
	Refresh       bool   `query:"refresh" doc:"Force on-demand AT poll instead of cached values"`
}

type statusResponseBody struct {
	Modem   statusModem   `json:"modem"`
	SIM     statusSIM     `json:"sim"`
	Network statusNetwork `json:"network"`
	Battery statusBattery `json:"battery"`
	UptimeS int64         `json:"uptime_s"`
	TS      string        `json:"ts" format:"date-time"`
}

type statusModem struct {
	Model    string `json:"model"`
	IMEI     string `json:"imei"`
	Firmware string `json:"firmware"`
	Epoch    int64  `json:"epoch"`
}

type statusSIM struct {
	State    string `json:"state" enum:"ready,pin_required,absent"`
	ICCID    string `json:"iccid"`
	IMSI     string `json:"imsi"`
	Operator string `json:"operator"`
}

type statusNetwork struct {
	Tech       string `json:"tech" enum:"LTE,UMTS,GSM,none"`
	Band       string `json:"band"`
	RSRPdBm    int    `json:"rsrp_dbm"`
	RSRQdB     int    `json:"rsrq_db"`
	CSQ        int    `json:"csq"`
	Registered bool   `json:"registered"`
}

type statusBattery struct {
	VoltageV float64 `json:"voltage_v"`
}

type statusGetOutput struct {
	Body statusResponseBody
}

func registerStatus(api huma.API, m modem.Modem) {
	huma.Register(api, huma.Operation{
		OperationID: "statusGet",
		Method:      http.MethodGet,
		Path:        "/v1/status",
		Summary:     "Get modem status",
		Tags:        []string{"status"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
	}, func(ctx context.Context, in *statusGetInput) (*statusGetOutput, error) {
		st, err := m.Status(ctx, in.Refresh)
		if err != nil {
			return nil, huma.Error503ServiceUnavailable("modem_not_ready: " + err.Error())
		}
		return &statusGetOutput{
			Body: statusResponseBody{
				Modem:   statusModem{Model: st.Model, IMEI: st.IMEI, Firmware: st.Firmware, Epoch: st.Epoch},
				SIM:     statusSIM{State: st.SIM.State, ICCID: st.SIM.ICCID, IMSI: st.SIM.IMSI, Operator: st.SIM.Operator},
				Network: statusNetwork{
					Tech: st.Network.Tech, Band: st.Network.Band,
					RSRPdBm: st.Network.RSRPdBm, RSRQdB: st.Network.RSRQdB,
					CSQ: st.Network.CSQ, Registered: st.Network.Registered,
				},
				Battery: statusBattery{VoltageV: st.BatteryV},
				UptimeS: st.UptimeSec,
				TS:      st.UpdatedAt.UTC().Format(time.RFC3339),
			},
		}, nil
	})
}
