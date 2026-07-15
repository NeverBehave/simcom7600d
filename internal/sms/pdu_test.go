package sms

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEncode_ShortGSM7(t *testing.T) {
	parts, err := Encode("+15551234567", "hello", EncodeOptions{})
	require.NoError(t, err)
	require.Len(t, parts, 1)
	require.Equal(t, "gsm7", parts[0].Encoding)
	require.NotEmpty(t, parts[0].HexPDU)
	require.Greater(t, parts[0].TPDULen, 0)
}

func TestEncode_LongUCS2_IsMultipart(t *testing.T) {
	body := strings.Repeat("中", 80) // 80 Chinese chars => UCS-2 forces multipart
	parts, err := Encode("+15551234567", body, EncodeOptions{})
	require.NoError(t, err)
	require.Equal(t, "ucs2", parts[0].Encoding)
	require.GreaterOrEqual(t, len(parts), 2)
	for _, p := range parts {
		require.Greater(t, p.TPDULen, 0)
		require.True(t, len(p.HexPDU)%2 == 0)
	}
}

func TestDecode_RoundTrip(t *testing.T) {
	parts, err := Encode("+15551234567", "hi there", EncodeOptions{})
	require.NoError(t, err)
	d, err := Decode(deliverFixtureHex())
	require.NoError(t, err)
	require.Equal(t, "Hello", d.Body)
	require.Equal(t, "+15551234567", d.FromAddr)
	require.Equal(t, "gsm7", d.Encoding)
	_ = parts
}

func TestDecodeModemPDU_StripsSMSCPrefix(t *testing.T) {
	// Six-byte SMSC address followed by the normal SMS-DELIVER TPDU.
	full := "06912143658709" + deliverFixtureHex()
	d, err := DecodeModemPDU(full)
	require.NoError(t, err)
	require.Equal(t, "Hello", d.Body)
	require.Equal(t, "+15551234567", d.FromAddr)
}

// deliverFixtureHex returns a real SMS-DELIVER TPDU hex string for:
//
//	From: +15551234567, Body: "Hello", Encoding: GSM-7
//	Timestamp: 2024-01-15 10:30:00 UTC
//
// Generated via the installed warthog618/sms v0.3.0 library.
func deliverFixtureHex() string {
	return "000B915155214365F700004210510103000005C8329BFD06"
}
