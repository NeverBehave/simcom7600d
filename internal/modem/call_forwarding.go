package modem

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"sim7600d/internal/atexec"
	"sim7600d/internal/atproto"
)

const voiceServiceClass = 1

var (
	ErrInvalidCallForwardingReason  = errors.New("modem: invalid call forwarding reason")
	ErrInvalidCallForwardingNumber  = errors.New("modem: forwarding number required when enabled")
	ErrInvalidCallForwardingTimeout = errors.New("modem: no-reply timeout must be 5, 10, 15, 20, 25, or 30 seconds")
)

var callForwardingReasonCodes = map[string]int{
	"unconditional": 0,
	"busy":          1,
	"no_reply":      2,
	"unreachable":   3,
}

var orderedCallForwardingReasons = []string{
	"unconditional", "busy", "no_reply", "unreachable",
}

// GetCallForwarding reads each individual condition. Querying reasons 4 or 5
// can collapse carrier state and does not reliably return the destination for
// every condition, so the four user-editable reasons are queried separately.
func (f *Facade) GetCallForwarding(ctx context.Context) ([]CallForwardingRule, error) {
	rules := make([]CallForwardingRule, 0, len(orderedCallForwardingReasons))
	for i, reason := range orderedCallForwardingReasons {
		rule, err := f.queryCallForwarding(ctx, reason)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			// Carrier supplementary-service failures generally apply to the
			// subscription, not one reason. Avoid making the browser wait through
			// four consecutive network timeouts and report the state as unknown.
			for _, remaining := range orderedCallForwardingReasons[i:] {
				rules = append(rules, CallForwardingRule{Reason: remaining, Error: err.Error()})
			}
			break
		}
		rules = append(rules, rule)
	}
	return rules, nil
}

// SetCallForwarding changes only the telephony service class and re-queries
// the network after a change so the API distinguishes confirmed state from an
// accepted update whose status cannot be queried. The UI suppresses unchanged
// writes when current state is available; doing a second preflight query here
// would make updates unusable on carriers that reject CCFC status queries.
func (f *Facade) SetCallForwarding(ctx context.Context, requested CallForwardingRule) (CallForwardingRule, error) {
	code, ok := callForwardingReasonCodes[requested.Reason]
	if !ok {
		return CallForwardingRule{}, ErrInvalidCallForwardingReason
	}
	if requested.Enabled && requested.Number == "" {
		return CallForwardingRule{}, ErrInvalidCallForwardingNumber
	}
	if requested.Reason == "no_reply" && requested.Enabled {
		if requested.TimeoutSeconds == 0 {
			requested.TimeoutSeconds = 20
		}
		if requested.TimeoutSeconds < 5 || requested.TimeoutSeconds > 30 || requested.TimeoutSeconds%5 != 0 {
			return CallForwardingRule{}, ErrInvalidCallForwardingTimeout
		}
	}

	var cmd string
	if !requested.Enabled {
		// Use the minimal form accepted by IMS-only carrier profiles. Some
		// SIM7600 firmware sends the optional empty address and class fields
		// through the legacy CS supplementary-service path, which times out on
		// LTE even though the same operation succeeds when those fields are
		// omitted.
		cmd = fmt.Sprintf("AT+CCFC=%d,0", code)
	} else {
		addressType := 129
		if strings.HasPrefix(requested.Number, "+") {
			addressType = 145
		}
		cmd = fmt.Sprintf(`AT+CCFC=%d,3,%q,%d,%d`, code, requested.Number, addressType, voiceServiceClass)
		if requested.Reason == "no_reply" {
			// Skip subaddress and subaddress type to reach the timeout field.
			cmd += fmt.Sprintf(",,,%d", requested.TimeoutSeconds)
		}
	}

	// T-Mobile's IMS supplementary-service update took a little over 30
	// seconds in live testing. Keep status queries short, but allow mutations
	// enough time to receive the carrier's final response.
	resp, err := f.ex.Exec(ctx, atexec.Cmd(cmd).WithTimeout(60*time.Second))
	if err != nil {
		return CallForwardingRule{}, err
	}
	if resp.Final.Final != atproto.FinalOK {
		// Some IMS carrier profiles apply the update but still finish the AT
		// transaction with a network-timeout error. Treat the network's queried
		// state as authoritative when it exactly matches the request.
		confirmed, confirmErr := f.queryCallForwarding(ctx, requested.Reason)
		if confirmErr == nil && callForwardingRuleMatches(requested, confirmed) {
			return confirmed, nil
		}
		if confirmErr != nil {
			return CallForwardingRule{}, fmt.Errorf("set call forwarding: %s; confirmation failed: %v", resp.Final.Line, confirmErr)
		}
		return CallForwardingRule{}, fmt.Errorf("set call forwarding: %s", resp.Final.Line)
	}
	confirmed, err := f.queryCallForwarding(ctx, requested.Reason)
	if err != nil {
		requested.Error = "update accepted; confirmation failed: " + err.Error()
		return requested, nil
	}
	return confirmed, nil
}

func callForwardingRuleMatches(requested, confirmed CallForwardingRule) bool {
	if !confirmed.Available || requested.Enabled != confirmed.Enabled {
		return false
	}
	if !requested.Enabled {
		return true
	}
	if requested.Number != confirmed.Number {
		return false
	}
	return requested.Reason != "no_reply" || requested.TimeoutSeconds == 0 || requested.TimeoutSeconds == confirmed.TimeoutSeconds
}

func (f *Facade) queryCallForwarding(ctx context.Context, reason string) (CallForwardingRule, error) {
	code, ok := callForwardingReasonCodes[reason]
	if !ok {
		return CallForwardingRule{}, ErrInvalidCallForwardingReason
	}
	resp, err := f.ex.Exec(ctx, atexec.Cmd(fmt.Sprintf("AT+CCFC=%d,2", code)).WithTimeout(20*time.Second))
	if err != nil {
		return CallForwardingRule{}, err
	}
	if resp.Final.Final != atproto.FinalOK {
		return CallForwardingRule{}, fmt.Errorf("query call forwarding: %s", resp.Final.Line)
	}

	result := CallForwardingRule{Reason: reason, Available: true}
	for _, line := range resp.Lines {
		record, ok := parseCCFCLine(line)
		if !ok || record.serviceClass&voiceServiceClass == 0 {
			continue
		}
		if record.enabled || result.Number == "" {
			result.Enabled = record.enabled
			result.Number = record.number
			result.TimeoutSeconds = record.timeoutSeconds
		}
		if record.enabled {
			break
		}
	}
	if reason == "no_reply" && result.TimeoutSeconds == 0 {
		result.TimeoutSeconds = 20
	}
	return result, nil
}

type ccfcRecord struct {
	enabled        bool
	serviceClass   int
	number         string
	timeoutSeconds int
}

func parseCCFCLine(line string) (ccfcRecord, bool) {
	if !strings.HasPrefix(line, "+CCFC:") {
		return ccfcRecord{}, false
	}
	r := csv.NewReader(strings.NewReader(strings.TrimSpace(strings.TrimPrefix(line, "+CCFC:"))))
	r.TrimLeadingSpace = true
	r.FieldsPerRecord = -1
	fields, err := r.Read()
	if err != nil || len(fields) < 2 {
		return ccfcRecord{}, false
	}
	status, err1 := strconv.Atoi(strings.TrimSpace(fields[0]))
	class, err2 := strconv.Atoi(strings.TrimSpace(fields[1]))
	if err1 != nil || err2 != nil {
		return ccfcRecord{}, false
	}
	record := ccfcRecord{enabled: status == 1, serviceClass: class}
	if len(fields) > 2 {
		record.number = strings.TrimSpace(fields[2])
	}
	// The timeout is the final field when returned for no-reply forwarding.
	if len(fields) > 6 {
		record.timeoutSeconds, _ = strconv.Atoi(strings.TrimSpace(fields[len(fields)-1]))
	}
	return record, true
}
