package atproto

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func framesOf(t *testing.T, raw string) []Frame {
	t.Helper()
	s := NewScanner(bytes.NewReader([]byte(raw)))
	var got []Frame
	for {
		f, err := s.Next()
		if err == ErrEOF {
			return got
		}
		require.NoError(t, err)
		got = append(got, f)
	}
}

func TestScanner_OKAfterIntermediate(t *testing.T) {
	got := framesOf(t, "\r\n+CSQ: 20,99\r\n\r\nOK\r\n")
	require.Equal(t, []Frame{
		{Kind: KindIntermediate, Line: "+CSQ: 20,99"},
		{Kind: KindFinal, Line: "OK", Final: FinalOK},
	}, got)
}

func TestScanner_CMEError(t *testing.T) {
	got := framesOf(t, "\r\n+CME ERROR: 10\r\n")
	require.Equal(t, []Frame{
		{Kind: KindFinal, Line: "+CME ERROR: 10", Final: FinalCMEError, Code: 10},
	}, got)
}

func TestScanner_CMSError(t *testing.T) {
	got := framesOf(t, "\r\n+CMS ERROR: 500\r\n")
	require.Equal(t, []Frame{
		{Kind: KindFinal, Line: "+CMS ERROR: 500", Final: FinalCMSError, Code: 500},
	}, got)
}

func TestScanner_PromptByte(t *testing.T) {
	got := framesOf(t, "\r\n> ")
	require.Equal(t, []Frame{{Kind: KindPrompt, Line: "> "}}, got)
}

func TestScanner_URC(t *testing.T) {
	got := framesOf(t, "\r\nRING\r\n\r\n+CMTI: \"ME\",5\r\n")
	require.Equal(t, []Frame{
		{Kind: KindURC, Line: "RING"},
		{Kind: KindURC, Line: "+CMTI: \"ME\",5"},
	}, got)
}

func TestScanner_NoCarrierIsFinal(t *testing.T) {
	got := framesOf(t, "\r\nNO CARRIER\r\n")
	require.Equal(t, []Frame{
		{Kind: KindFinal, Line: "NO CARRIER", Final: FinalNoCarrier},
	}, got)
}

func TestErrorMeanings(t *testing.T) {
	require.Equal(t, "SIM not inserted",          CMEMeaning(10))
	require.Equal(t, "SIM PIN required",          CMEMeaning(11))
	require.Equal(t, "unknown CME code 9999",     CMEMeaning(9999))
	require.Equal(t, "unknown",                   CMSMeaning(500))
}
