# SIM7600 Android app

The native Android client exposes the complete `sim7600d` user-facing feature
set:

- modem, SIM, radio, signal, and recent-event status;
- threaded inbound/outbound SMS with sending, deletion, delivery state, and
  message details;
- incoming/outgoing call history, dialing, answer, reject, hangup, hold,
  resume, conference merge, DTMF, callback, and call details;
- full-duplex 16 kHz PCM call audio using the phone microphone and speaker,
  including mute and test-tone controls;
- carrier call-forwarding status and updates;
- visual voicemail listing, authenticated playback, heard state, callback,
  deletion, manual sync, and new-voicemail notifications;
- event filtering and administrator reconcile, queue, VACUUM, modem reset, and
  optional AT-console operations;
- an authenticated foreground event stream with high-priority incoming-call
  notifications, Answer/Reject actions, SMS previews, reconnect/resume, and
  automatic restart after device reboot.

## Build

Requirements:

- JDK 17
- Android SDK Platform 35
- Android SDK Build Tools 35

The repository pins Gradle through its wrapper:

```sh
export JAVA_HOME=/path/to/jdk-17
cd android
./gradlew testDebugUnitTest lintDebug assembleDebug
```

On Homebrew Apple Silicon installations used for this project:

```sh
export JAVA_HOME=/opt/homebrew/opt/openjdk@17/libexec/openjdk.jdk/Contents/Home
export ANDROID_HOME=/opt/homebrew/share/android-commandlinetools
```

Install the debug build on a connected device:

```sh
adb install -r app/build/outputs/apk/debug/app-debug.apk
```

## Connect

Enter the externally reachable HTTPS server address and the same bearer token
used by the web UI. A Cloudflare Tunnel hostname is recommended away from the
home network. The token remains in the app-private preferences and Android
cloud backup is disabled.

For development through the Mac's existing SSH tunnel, attach an Android device
with USB debugging and reverse the port:

```sh
adb reverse tcp:18080 tcp:18080
```

Then use `http://127.0.0.1:18080` as the app's server address. Android cleartext
traffic is enabled for this local workflow; production should use HTTPS.

## Real-time notifications

After sign-in, grant notification permission. The app starts a foreground
`remoteMessaging` service and connects to the authenticated
`GET /v1/events/stream` Server-Sent Events endpoint. The server sends new events
within roughly one second and heartbeat frames keep proxies from idling the
connection. The app stores the last event ID so reconnects recover events that
arrived during a temporary network interruption without replaying old history.

Android displays a low-priority persistent notification while this connection
is active. Incoming calls use the high-priority call channel and offer Answer
and Reject actions. SMS notifications include the sender and message preview;
tapping one opens its conversation. Call and message details are marked private
so the lock screen can hide them according to the device's notification policy.

Some Android vendors apply additional battery restrictions. If alerts stop
after long idle periods, allow unrestricted background battery use for
SIM7600 in the device's app settings.
