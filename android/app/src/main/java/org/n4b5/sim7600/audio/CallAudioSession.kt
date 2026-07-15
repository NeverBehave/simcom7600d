package org.n4b5.sim7600.audio

import android.content.Context
import android.media.AudioAttributes
import android.media.AudioFormat
import android.media.AudioManager
import android.media.AudioRecord
import android.media.AudioTrack
import android.media.MediaRecorder
import android.media.audiofx.AcousticEchoCanceler
import android.media.audiofx.NoiseSuppressor
import android.os.Handler
import android.os.Looper
import okhttp3.Response
import okhttp3.WebSocket
import okhttp3.WebSocketListener
import okio.ByteString
import org.json.JSONObject
import org.n4b5.sim7600.network.ApiClient
import java.nio.ByteBuffer
import java.nio.ByteOrder
import java.util.concurrent.atomic.AtomicBoolean
import kotlin.concurrent.thread
import kotlin.math.max
import kotlin.math.PI
import kotlin.math.sin

enum class AudioState { CONNECTING, LIVE, STOPPED, ERROR }

class CallAudioSession(
    private val context: Context,
    private val client: ApiClient,
    private val callId: String,
    private val onState: (AudioState, String) -> Unit,
) {
    private val running = AtomicBoolean(false)
    private val main = Handler(Looper.getMainLooper())
    private var socket: WebSocket? = null
    private var recorder: AudioRecord? = null
    private var player: AudioTrack? = null
    private var echoCanceler: AcousticEchoCanceler? = null
    private var noiseSuppressor: NoiseSuppressor? = null
    @Volatile var microphoneMuted = false
    @Volatile var speakerMuted = false
    @Volatile private var toneSamplesRemaining = 0
    private var tonePhase = 0.0

    fun start() {
        if (!running.compareAndSet(false, true)) return
        post(AudioState.CONNECTING, "Connecting call audio…")
        socket = client.openAudioWebSocket(callId, object : WebSocketListener() {
            override fun onMessage(webSocket: WebSocket, text: String) {
                val ready = runCatching {
                    val json = JSONObject(text)
                    json.optString("type") == "ready" && json.optInt("sample_rate") == SAMPLE_RATE
                }.getOrDefault(false)
                if (ready) startAudio(webSocket) else fail("Invalid call-audio handshake")
            }

            override fun onMessage(webSocket: WebSocket, bytes: ByteString) {
                if (!speakerMuted) player?.write(bytes.toByteArray(), 0, bytes.size)
            }

            override fun onFailure(webSocket: WebSocket, t: Throwable, response: Response?) {
                fail(response?.message?.ifBlank { null } ?: t.message ?: "Call audio disconnected")
            }

            override fun onClosed(webSocket: WebSocket, code: Int, reason: String) {
                stopInternal()
                post(AudioState.STOPPED, reason.ifBlank { "Call audio disconnected" })
            }
        })
    }

    fun stop() {
        socket?.close(1000, "Android audio disconnected")
        stopInternal()
        post(AudioState.STOPPED, "Call audio disconnected")
    }

    fun sendTestTone() {
        tonePhase = 0.0
        toneSamplesRemaining = SAMPLE_RATE
    }

    @Suppress("MissingPermission")
    private fun startAudio(webSocket: WebSocket) {
        if (!running.get() || recorder != null) return
        try {
            val inputMin = AudioRecord.getMinBufferSize(SAMPLE_RATE, AudioFormat.CHANNEL_IN_MONO, AudioFormat.ENCODING_PCM_16BIT)
            val outputMin = AudioTrack.getMinBufferSize(SAMPLE_RATE, AudioFormat.CHANNEL_OUT_MONO, AudioFormat.ENCODING_PCM_16BIT)
            val inputBuffer = max(inputMin, FRAME_BYTES * 4)
            recorder = AudioRecord(
                MediaRecorder.AudioSource.VOICE_COMMUNICATION,
                SAMPLE_RATE,
                AudioFormat.CHANNEL_IN_MONO,
                AudioFormat.ENCODING_PCM_16BIT,
                inputBuffer,
            )
            check(recorder?.state == AudioRecord.STATE_INITIALIZED) { "Microphone does not support 16 kHz PCM" }
            player = AudioTrack.Builder()
                .setAudioAttributes(
                    AudioAttributes.Builder().setUsage(AudioAttributes.USAGE_VOICE_COMMUNICATION)
                        .setContentType(AudioAttributes.CONTENT_TYPE_SPEECH).build(),
                )
                .setAudioFormat(
                    AudioFormat.Builder().setSampleRate(SAMPLE_RATE).setEncoding(AudioFormat.ENCODING_PCM_16BIT)
                        .setChannelMask(AudioFormat.CHANNEL_OUT_MONO).build(),
                )
                .setBufferSizeInBytes(max(outputMin, FRAME_BYTES * 4))
                .setTransferMode(AudioTrack.MODE_STREAM)
                .build()
            val audioManager = context.getSystemService(AudioManager::class.java)
            audioManager.mode = AudioManager.MODE_IN_COMMUNICATION
            @Suppress("DEPRECATION")
            run { audioManager.isSpeakerphoneOn = true }
            recorder?.audioSessionId?.takeIf { it > 0 }?.let { session ->
                if (AcousticEchoCanceler.isAvailable()) echoCanceler = AcousticEchoCanceler.create(session)?.apply { enabled = true }
                if (NoiseSuppressor.isAvailable()) noiseSuppressor = NoiseSuppressor.create(session)?.apply { enabled = true }
            }
            player?.play()
            recorder?.startRecording()
            post(AudioState.LIVE, "Call audio connected")
            thread(name = "sim7600-microphone", isDaemon = true) { captureLoop(webSocket) }
        } catch (error: Throwable) {
            fail(error.message ?: "Could not start call audio")
        }
    }

    private fun captureLoop(webSocket: WebSocket) {
        val shorts = ShortArray(FRAME_SAMPLES)
        while (running.get()) {
            val count = recorder?.read(shorts, 0, shorts.size) ?: break
            if (count <= 0) continue
            val bytes = ByteBuffer.allocate(count * 2).order(ByteOrder.LITTLE_ENDIAN)
            repeat(count) { index ->
                val sample = when {
                    toneSamplesRemaining > 0 -> {
                        val value = (sin(tonePhase) * 0.15 * Short.MAX_VALUE).toInt().toShort()
                        tonePhase = (tonePhase + 2 * PI * 440 / SAMPLE_RATE) % (2 * PI)
                        toneSamplesRemaining -= 1
                        value
                    }
                    microphoneMuted -> 0.toShort()
                    else -> shorts[index]
                }
                bytes.putShort(sample)
            }
            if (webSocket.queueSize() < 64 * 1024) webSocket.send(ByteString.of(*bytes.array()))
        }
    }

    private fun fail(message: String) {
        if (!running.getAndSet(false)) return
        socket?.cancel()
        releaseAudio()
        post(AudioState.ERROR, message)
    }

    private fun stopInternal() {
        if (!running.getAndSet(false)) return
        releaseAudio()
    }

    private fun releaseAudio() {
        runCatching { recorder?.stop() }
        runCatching { player?.stop() }
        recorder?.release(); recorder = null
        player?.release(); player = null
        echoCanceler?.release(); echoCanceler = null
        noiseSuppressor?.release(); noiseSuppressor = null
        context.getSystemService(AudioManager::class.java).mode = AudioManager.MODE_NORMAL
    }

    private fun post(state: AudioState, message: String) = main.post { onState(state, message) }

    companion object {
        const val SAMPLE_RATE = 16_000
        private const val FRAME_SAMPLES = SAMPLE_RATE * 20 / 1000
        private const val FRAME_BYTES = FRAME_SAMPLES * 2
    }
}
