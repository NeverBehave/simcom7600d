package atproto

import "fmt"

// cmeErrors maps the most common +CME ERROR numeric codes to a short, human
// description. Reference: 3GPP TS 27.007 §9.2. We don't need to be exhaustive —
// unknown codes are surfaced verbatim.
var cmeErrors = map[int]string{
	0:   "phone failure",
	3:   "operation not allowed",
	4:   "operation not supported",
	10:  "SIM not inserted",
	11:  "SIM PIN required",
	12:  "SIM PUK required",
	13:  "SIM failure",
	14:  "SIM busy",
	16:  "incorrect password",
	17:  "SIM PIN2 required",
	18:  "SIM PUK2 required",
	20:  "memory full",
	21:  "invalid index",
	22:  "not found",
	30:  "no network service",
	31:  "network timeout",
	32:  "network not allowed",
	100: "unknown",
}

var cmsErrors = map[int]string{
	300: "ME failure",
	301: "SMS service of ME reserved",
	302: "operation not allowed",
	303: "operation not supported",
	304: "invalid PDU mode parameter",
	305: "invalid text mode parameter",
	310: "SIM not inserted",
	311: "SIM PIN required",
	321: "invalid memory index",
	322: "memory full",
	330: "SMSC address unknown",
	331: "no network service",
	332: "network timeout",
	500: "unknown",
}

// CMEMeaning returns a short human description of a +CME ERROR code.
func CMEMeaning(code int) string {
	if s, ok := cmeErrors[code]; ok {
		return s
	}
	return fmt.Sprintf("unknown CME code %d", code)
}

// CMSMeaning returns a short human description of a +CMS ERROR code.
func CMSMeaning(code int) string {
	if s, ok := cmsErrors[code]; ok {
		return s
	}
	return fmt.Sprintf("unknown CMS code %d", code)
}
