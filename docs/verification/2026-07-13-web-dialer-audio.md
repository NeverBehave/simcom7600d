# Web dialer audio verification

Date: 2026-07-13–2026-07-14
Environment: `home-nas`, accessed through `http://localhost:18080`
Authorized live-test destination: redacted.

## Intended call path

1. The browser captures an echo-cancelled microphone stream.
2. An AudioWorklet batches microphone samples without blocking the page.
3. The UI downsamples browser audio to signed 16-bit, mono, 16 kHz PCM.
4. An authenticated WebSocket carries PCM frames in both directions.
5. Before each audio attachment, the daemon applies the SIMCom-documented `AT+CPCMFRM=1` 16 kHz mode, keeps the internal VoLTE PCM profile wideband, starts USB audio with `AT+CPCMREG=1`, and bridges the WebSocket to PID `9011`, interface `:1.6` (`/dev/ttyUSB4`).
6. Received modem PCM is scheduled into the browser's audio output with a small jitter buffer.

The token is sent as a WebSocket subprotocol rather than in the URL. Only one browser can own the modem audio port at a time.

## Verification matrix

| Case | Status | Evidence |
|---|---|---|
| Existing call control still dials and hangs up | Passed live | The test call progressed through alerting/active and ended from the new UI; its identifier is redacted. |
| Audio route rejects missing credentials | Passed live | Unauthenticated request returned HTTP 401. |
| Audio can attach only to an eligible call | Passed automated | Inactive-call bridge test returns HTTP 409 without opening the device. |
| Exclusive browser ownership | Passed automated | Bridge serializes ownership of the single modem audio interface. |
| Browser-to-modem PCM | Passed live | Probe wrote 127,680 bytes over eight seconds at the expected 16,000 bytes/s. |
| Modem-to-browser PCM | Passed live transport | Probe read 127,680 bytes over the same interval. The unanswered test leg contained silence, so audible downlink still requires a human answered-call check. |
| PCM format and sample-rate conversion | Passed automated | Streaming 48 kHz and 44.1 kHz conversion tests produce continuous 8 kHz signed PCM; clipping and fractional-rate boundary drift are covered. |
| Initial 16 kHz raw USB experiment | Failed uplink | The echo test sent 61,760 nonzero generated tone samples while the browser meter and 16 kHz WebSocket were active, but the echo return remained digital silence (RMS about 1, no tone energy). The raw serial audio path was temporarily restored to 8 kHz. |
| 8 kHz uplink rollback | Passed live | After modem reboot restored `CPCMFRM: 0`, the echo test returned the generated 1 kHz browser-side tone with peak detected amplitude 5,314 and RMS 3,768. This proves browser/WebSocket → raw USB audio → cellular caller uplink is working. |
| Documented 16 kHz call-start sequence | Passed live | The daemon now sends `AT+CPCMFRM=1` before `AT+CPCMREG=1` for every audio attachment and advertises 16 kHz to the browser. The echo test produced a 43% uplink test-tone peak and a 27% returned downlink peak; the Admin AT console then confirmed `+CPCMFRM: 1`. |
| Microphone permission and missing-device feedback | Passed live UI | Real browser showed the audio control during ringback and clearly reported that this Mac has no microphone. |
| Listen-only fallback and generated uplink test tone | Passed live | The call reached `Audio connected` without a microphone. The generated one-second tone registered 43% on the browser-to-modem meter while non-silent return audio registered 17% on the modem-to-browser meter. Audible tone confirmation remains human-gated. |
| Browser audio renderer never finishes starting | Passed regression/live | A test on a device-less Mac left `AudioContext.resume()` pending. Transport is now independent of renderer startup; an automated hung-resume test and the subsequent live call both pass. |
| Authenticated page reload | Passed regression | SDK configuration now occurs before authenticated children render, preventing a persisted-token refresh from leaving Calls stuck on `SDK not configured`. |
| Mute microphone, mute speaker, activity meters, disconnect/retry | Passed implementation/tests | Controls update the active audio session; directional meters expose microphone/caller activity; teardown releases media tracks, Web Audio nodes, WebSocket, modem PCM mode, and USB device. |
| Visible and effective Hang up control | Passed live | Tailwind v4 semantic colors are now mapped correctly. `ATH` was also replaced with unconditional voice release `AT+CHUP` because SIM7600 can ignore `ATH` while returning `OK` when `AT+CVHU=1`. The test call ended from the web in 656 ms and the reconciler still showed zero active calls after 3.5 seconds. |
| DTMF keypad request path | Passed live/API | The same answered call returned `Sent 5`. Successful HTTP 204 responses are no longer misclassified as empty-response failures, and `AT+VTS` now includes an explicit 200 ms (`2` tenths) duration. Audible remote confirmation remains user-gated. |
| Full remote speech in both directions | Pending human confirmation | Bidirectional non-silent PCM is proven in an answered call. Audible conversation still requires confirmation from the phone plus a browser host with a real microphone and speaker/headset. |

## Automated checks

- Complete Go suite, including the bidirectional WebSocket/PCM bridge.
- UI TypeScript check, production build, and 27 UI tests.
- NixOS package build using checked-in vendored dependencies.

## Hardware findings

- The current SIM7600 USB PID is `1e0e:9011`.
- Interface `:1.5` is the AT port; `:1.6` is the vendor raw USB Audio port.
- The audio port is not an ALSA/USB Audio Class device. It is a raw full-duplex PCM character device handled by the modem's USB serial driver.
- The service account belongs to `dialout`, the device is `root:dialout`, and systemd allows USB-serial major 188.
- This SIM7600G-H firmware advertises 8 kHz and 16 kHz USB/PCM modes and rejects explicit 32 kHz values. Its raw USB-serial interface now has verified bidirectional 16 kHz transport when `AT+CPCMFRM=1` is applied immediately before `AT+CPCMREG=1`; the setting is volatile and is therefore reapplied for every browser audio attachment.
