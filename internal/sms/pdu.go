// Package sms wraps github.com/warthog618/sms with the small surface the
// sim7600d facade needs. No other package in this module should import the
// third-party library directly.
package sms

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	wsms "github.com/warthog618/sms"
	"github.com/warthog618/sms/encoding/tpdu"
)

// Part is one PDU segment ready to hand to the modem via AT+CMGS.
type Part struct {
	Index    int    // 1-based segment index
	Total    int    // total number of segments in the message
	Encoding string // "gsm7" | "ucs2" | "8bit"
	HexPDU   string // hex-encoded TPDU bytes (uppercase)
	TPDULen  int    // byte length of the TPDU — what AT+CMGS=N expects
}

// Delivered represents a decoded SMS-DELIVER PDU.
type Delivered struct {
	FromAddr string
	Body     string
	Encoding string
	SMSCTime time.Time
	UDH      *Concat // non-nil iff this delivery is a concatenated SMS part
}

// Concat holds the concatenation information extracted from the UDH.
type Concat struct{ Ref, Total, Seq int }

// EncodeOptions configures SMS encoding behaviour.
type EncodeOptions struct {
	// StatusReport requests a delivery report (TP-SRR bit).
	StatusReport bool
}

// Encode encodes body as one or more SMS-SUBMIT PDUs addressed to to.
//
// The encoding is chosen automatically: GSM-7 for messages that fit in the
// default charset, UCS-2 otherwise. Long messages are split into concatenated
// segments automatically.
func Encode(to, body string, opts EncodeOptions) ([]Part, error) {
	to = strings.TrimSpace(to)
	if to == "" || body == "" {
		return nil, errors.New("sms: to and body required")
	}

	encOpts := []wsms.EncoderOption{wsms.To(to), wsms.AsSubmit}
	if opts.StatusReport {
		// Set the TP-SRR bit via a template TPDU with FoSRR set.
		tmpl := tpdu.TPDU{}
		_ = tpdu.SmsSubmit.ApplyTPDUOption(&tmpl)
		tmpl.FirstOctet |= tpdu.FoSRR
		encOpts = append(encOpts, wsms.WithTemplate(tmpl))
	}

	tpdus, err := wsms.Encode([]byte(body), encOpts...)
	if err != nil {
		return nil, fmt.Errorf("sms: encode: %w", err)
	}
	if len(tpdus) == 0 {
		return nil, errors.New("sms: encoder produced no TPDUs")
	}

	out := make([]Part, len(tpdus))
	for i := range tpdus {
		raw, err := tpdus[i].MarshalBinary()
		if err != nil {
			return nil, fmt.Errorf("sms: marshal TPDU %d: %w", i, err)
		}
		alpha, _ := tpdus[i].Alphabet()
		out[i] = Part{
			Index:    i + 1,
			Total:    len(tpdus),
			Encoding: alphaToEncoding(alpha),
			HexPDU:   strings.ToUpper(hex.EncodeToString(raw)),
			TPDULen:  len(raw),
		}
	}
	return out, nil
}

// Decode decodes a hex-encoded SMS-DELIVER TPDU.
//
// hexPDU must be the raw TPDU bytes (no SMSC address prefix).
func Decode(hexPDU string) (Delivered, error) {
	raw, err := hex.DecodeString(strings.TrimSpace(hexPDU))
	if err != nil {
		return Delivered{}, fmt.Errorf("sms: invalid hex: %w", err)
	}
	return decodeTPDU(raw)
}

// DecodeModemPDU decodes the complete PDU returned by AT+CMGR/AT+CMGL. The
// first octet is the SMSC address length and is not part of the TPDU.
func DecodeModemPDU(hexPDU string) (Delivered, error) {
	raw, err := hex.DecodeString(strings.TrimSpace(hexPDU))
	if err != nil {
		return Delivered{}, fmt.Errorf("sms: invalid hex: %w", err)
	}
	if len(raw) < 2 {
		return Delivered{}, errors.New("sms: complete PDU is too short")
	}
	tpduStart := 1 + int(raw[0])
	if tpduStart >= len(raw) {
		return Delivered{}, errors.New("sms: invalid SMSC prefix length")
	}
	return decodeTPDU(raw[tpduStart:])
}

func decodeTPDU(raw []byte) (Delivered, error) {

	// Unmarshal as MT (mobile-terminated) so the direction is SmsDeliver.
	pdu, err := wsms.Unmarshal(raw, wsms.AsMT)
	if err != nil {
		return Delivered{}, fmt.Errorf("sms: unmarshal: %w", err)
	}

	alpha, err := pdu.Alphabet()
	if err != nil {
		return Delivered{}, fmt.Errorf("sms: alphabet: %w", err)
	}

	body, err := tpdu.DecodeUserData(pdu.UD, pdu.UDH, alpha, tpdu.WithAllCharsets)
	if err != nil {
		return Delivered{}, fmt.Errorf("sms: decode user data: %w", err)
	}

	d := Delivered{
		FromAddr: normalizeAddr(pdu.OA),
		Body:     string(body),
		Encoding: alphaToEncoding(alpha),
	}
	if !pdu.SCTS.IsZero() {
		d.SMSCTime = pdu.SCTS.UTC()
	}
	if c := udhConcat(pdu.UDH); c != nil {
		d.UDH = c
	}
	return d, nil
}

// DedupeKey returns a short deterministic string that identifies a message
// uniquely for deduplication purposes.
func DedupeKey(from string, ts time.Time, body string) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%s|%d|%s", from, ts.UnixNano(), body)))
	return hex.EncodeToString(h[:16])
}

// alphaToEncoding converts a tpdu.Alphabet value to our canonical string.
func alphaToEncoding(a tpdu.Alphabet) string {
	switch a {
	case tpdu.AlphaUCS2:
		return "ucs2"
	case tpdu.Alpha8Bit:
		return "8bit"
	default:
		return "gsm7"
	}
}

// normalizeAddr converts a TPDU Address to a normalised E.164 string.
//
// The Address.Number() method already prepends "+" for international numbers
// (TON == TonInternational), so we simply return it.
func normalizeAddr(a tpdu.Address) string {
	return a.Number()
}

// udhConcat extracts concatenation information from a UserDataHeader.
func udhConcat(udh tpdu.UserDataHeader) *Concat {
	// 8-bit reference (IE ID 0x00)
	if ie, ok := udh.IE(0x00); ok && len(ie.Data) == 3 {
		return &Concat{
			Ref:   int(ie.Data[0]),
			Total: int(ie.Data[1]),
			Seq:   int(ie.Data[2]),
		}
	}
	// 16-bit reference (IE ID 0x08)
	if ie, ok := udh.IE(0x08); ok && len(ie.Data) == 4 {
		return &Concat{
			Ref:   int(ie.Data[0])<<8 | int(ie.Data[1]),
			Total: int(ie.Data[2]),
			Seq:   int(ie.Data[3]),
		}
	}
	return nil
}
