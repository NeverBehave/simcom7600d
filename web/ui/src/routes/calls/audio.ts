export const CALL_AUDIO_PROTOCOL = 'sim7600.audio.v1';
export const MODEM_SAMPLE_RATE = 16000;

export type CallAudioState = 'requesting' | 'connecting' | 'live' | 'disconnected' | 'error';

type StateHandler = (state: CallAudioState) => void;
type ErrorHandler = (message: string) => void;
export type CallAudioLevels = { microphone: number; caller: number };
type LevelHandler = (levels: CallAudioLevels) => void;
type WarningHandler = (message: string) => void;
type MicrophoneHandler = (available: boolean) => void;

// Streaming box-filter downsampling avoids the harsh aliasing produced by
// selecting every sixth browser sample. State is retained between worklet
// chunks so there are no clicks at frame boundaries.
export class StreamingPCMEncoder {
  private readonly ratio: number;
  private carry = new Float32Array(0);
  private position = 0;

  constructor(sourceRate: number, targetRate = MODEM_SAMPLE_RATE) {
    if (!(sourceRate >= targetRate) || targetRate <= 0) {
      throw new Error(`unsupported audio sample rates: ${sourceRate} -> ${targetRate}`);
    }
    this.ratio = sourceRate / targetRate;
  }

  encode(input: Float32Array): Int16Array {
    const data = new Float32Array(this.carry.length + input.length);
    data.set(this.carry);
    data.set(input, this.carry.length);
    const values: number[] = [];

    // Permit tiny floating-point drift at exact chunk boundaries; otherwise
    // 44.1 kHz input can occasionally lose one 16 kHz output sample.
    while (this.position + this.ratio <= data.length + 1e-9) {
      const start = Math.floor(this.position);
      const end = Math.min(data.length, Math.max(start + 1, Math.floor(this.position + this.ratio)));
      let sum = 0;
      for (let i = start; i < end; i += 1) sum += data[i];
      const sample = Math.max(-1, Math.min(1, sum / (end - start)));
      values.push(sample < 0 ? Math.round(sample * 32768) : Math.round(sample * 32767));
      this.position += this.ratio;
    }

    const consumed = Math.floor(this.position);
    this.carry = data.slice(consumed);
    this.position -= consumed;
    return Int16Array.from(values);
  }
}

type BiquadCoefficients = {
  b0: number;
  b1: number;
  b2: number;
  a1: number;
  a2: number;
};

class StreamingBiquad {
  private x1 = 0;
  private x2 = 0;
  private y1 = 0;
  private y2 = 0;

  constructor(private readonly coefficients: BiquadCoefficients) {}

  process(sample: number): number {
    const { b0, b1, b2, a1, a2 } = this.coefficients;
    const output = b0 * sample + b1 * this.x1 + b2 * this.x2 - a1 * this.y1 - a2 * this.y2;
    this.x2 = this.x1;
    this.x1 = sample;
    this.y2 = this.y1;
    this.y1 = output;
    return output;
  }

  reset() {
    this.x1 = 0;
    this.x2 = 0;
    this.y1 = 0;
    this.y2 = 0;
  }
}

// The modem's wideband raw-audio mode runs at 16 kHz. A modest upper-speech
// shelf improves consonant presence while retaining frequencies through the
// stream's 8 kHz Nyquist limit. State is retained across WebSocket frames to
// avoid filter-boundary clicks.
export class StreamingSpeechEnhancer {
  private readonly highPass: StreamingBiquad;
  private readonly presenceShelf: StreamingBiquad;

  constructor(sampleRate = MODEM_SAMPLE_RATE) {
    if (sampleRate <= 0) throw new Error(`invalid speech sample rate: ${sampleRate}`);
    this.highPass = new StreamingBiquad(highPassCoefficients(sampleRate, 100));
    this.presenceShelf = new StreamingBiquad(highShelfCoefficients(sampleRate, 1800, 4));
  }

  process(samples: Int16Array): Int16Array {
    const output = new Int16Array(samples.length);
    for (let i = 0; i < samples.length; i += 1) {
      const normalized = samples[i] / 32768;
      const filtered = this.presenceShelf.process(this.highPass.process(normalized));
      const magnitude = Math.abs(filtered);
      // A gentle 5:1 peak limiter only acts near full scale. This leaves normal
      // speech dynamics intact while containing the shelf's maximum gain.
      const limitedMagnitude = magnitude > 0.82
        ? Math.min(0.98, 0.82 + (magnitude - 0.82) / 5)
        : magnitude;
      const limited = Math.sign(filtered) * limitedMagnitude;
      output[i] = limited < 0
        ? Math.round(limited * 32768)
        : Math.round(limited * 32767);
    }
    return output;
  }

  reset() {
    this.highPass.reset();
    this.presenceShelf.reset();
  }
}

function highPassCoefficients(sampleRate: number, frequency: number): BiquadCoefficients {
  const omega = (2 * Math.PI * frequency) / sampleRate;
  const cosine = Math.cos(omega);
  const alpha = Math.sin(omega) / (2 * Math.SQRT1_2);
  const a0 = 1 + alpha;
  return {
    b0: ((1 + cosine) / 2) / a0,
    b1: (-(1 + cosine)) / a0,
    b2: ((1 + cosine) / 2) / a0,
    a1: (-2 * cosine) / a0,
    a2: (1 - alpha) / a0,
  };
}

function highShelfCoefficients(sampleRate: number, frequency: number, gainDB: number): BiquadCoefficients {
  const amplitude = 10 ** (gainDB / 40);
  const omega = (2 * Math.PI * frequency) / sampleRate;
  const cosine = Math.cos(omega);
  const alpha = (Math.sin(omega) / 2) * Math.SQRT2;
  const twoRootAAlpha = 2 * Math.sqrt(amplitude) * alpha;
  const a0 = (amplitude + 1) - (amplitude - 1) * cosine + twoRootAAlpha;
  return {
    b0: (amplitude * ((amplitude + 1) + (amplitude - 1) * cosine + twoRootAAlpha)) / a0,
    b1: (-2 * amplitude * ((amplitude - 1) + (amplitude + 1) * cosine)) / a0,
    b2: (amplitude * ((amplitude + 1) + (amplitude - 1) * cosine - twoRootAAlpha)) / a0,
    a1: (2 * ((amplitude - 1) - (amplitude + 1) * cosine)) / a0,
    a2: ((amplitude + 1) - (amplitude - 1) * cosine - twoRootAAlpha) / a0,
  };
}

class PCMPlayer {
  private readonly gain: GainNode;
  private readonly enhancer = new StreamingSpeechEnhancer();
  private nextStart = 0;

  constructor(private readonly context: AudioContext) {
    this.gain = context.createGain();
    this.gain.connect(context.destination);
  }

  setMuted(muted: boolean) {
    this.gain.gain.setValueAtTime(muted ? 0 : 1, this.context.currentTime);
  }

  enqueue(bytes: ArrayBuffer, sampleRate: number) {
    if (bytes.byteLength < 2 || bytes.byteLength % 2 !== 0) return;
    const samples = bytes.byteLength / 2;
    const audioBuffer = this.context.createBuffer(1, samples, sampleRate);
    const output = audioBuffer.getChannelData(0);
    const view = new DataView(bytes);
    const input = new Int16Array(samples);
    for (let i = 0; i < samples; i += 1) {
      input[i] = view.getInt16(i * 2, true);
    }
    const enhanced = this.enhancer.process(input);
    for (let i = 0; i < samples; i += 1) {
      output[i] = enhanced[i] / 32768;
    }

    const source = this.context.createBufferSource();
    source.buffer = audioBuffer;
    source.connect(this.gain);
    const now = this.context.currentTime;
    if (this.nextStart < now + 0.04 || this.nextStart > now + 0.5) {
      this.nextStart = now + 0.04;
    }
    source.start(this.nextStart);
    this.nextStart += audioBuffer.duration;
  }

  disconnect() {
    this.gain.disconnect();
  }
}

export function callAudioWebSocketURL(location: Pick<Location, 'protocol' | 'host'>, callID: string): string {
  const scheme = location.protocol === 'https:' ? 'wss:' : 'ws:';
  return `${scheme}//${location.host}/v1/calls/${encodeURIComponent(callID)}/audio`;
}

export class BrowserCallAudioSession {
  private disposed = false;
  private ready = false;
  private micMuted = false;
  private speakerMuted = false;
  private stream?: MediaStream;
  private context?: AudioContext;
  private socket?: WebSocket;
  private capture?: AudioWorkletNode;
  private microphone?: MediaStreamAudioSourceNode;
  private silentOutput?: GainNode;
  private player?: PCMPlayer;
  private encoder?: StreamingPCMEncoder;
  private readonly uplinkEnhancer = new StreamingSpeechEnhancer();
  private levels: CallAudioLevels = { microphone: 0, caller: 0 };
  private lastLevelUpdate = 0;
  private syntheticTimer?: number;
  private toneSamplesRemaining = 0;
  private tonePhase = 0;

  constructor(
    private readonly callID: string,
    private readonly token: string,
    private readonly onState: StateHandler,
    private readonly onError: ErrorHandler,
    private readonly onLevels: LevelHandler = () => undefined,
    private readonly onWarning: WarningHandler = () => undefined,
    private readonly onMicrophone: MicrophoneHandler = () => undefined,
  ) {}

  async start() {
    try {
      this.onState('requesting');
      if ((!window.isSecureContext && window.location.hostname !== 'localhost') || !navigator.mediaDevices?.getUserMedia) {
        this.onWarning('Microphone access is unavailable. Call audio will connect in listen-only mode.');
        this.onMicrophone(false);
      } else {
        try {
          this.stream = await navigator.mediaDevices.getUserMedia({
            audio: { echoCancellation: true, noiseSuppression: true, autoGainControl: true },
            video: false,
          });
          this.onMicrophone(true);
        } catch (error) {
          this.onWarning(`${audioErrorMessage(error)} Call audio will connect in listen-only mode.`);
          this.onMicrophone(false);
        }
      }
      if (this.disposed) {
        this.stream?.getTracks().forEach((track) => track.stop());
        return;
      }

      this.context = new AudioContext({ latencyHint: 'interactive' });
      this.player = new PCMPlayer(this.context);
      this.player.setMuted(this.speakerMuted);

      // Some browsers leave resume() pending forever when the computer has no
      // audio output device. Transport, receive metering, and the synthetic
      // test tone do not depend on it, so never hold the WebSocket connection
      // behind the browser audio renderer becoming runnable.
      void this.context.resume().catch((error) => {
        if (!this.disposed) {
          this.onWarning(`Browser audio output could not start: ${audioErrorMessage(error)}`);
        }
      });

      if (this.stream) {
        await this.context.audioWorklet.addModule('/audio-capture-worklet.js');
        this.encoder = new StreamingPCMEncoder(this.context.sampleRate);
        this.microphone = this.context.createMediaStreamSource(this.stream);
        this.capture = new AudioWorkletNode(this.context, 'sim7600-microphone-capture', {
          numberOfInputs: 1,
          numberOfOutputs: 1,
          outputChannelCount: [1],
        });
        // Keep the worklet in the rendering graph without echoing the local mic.
        this.silentOutput = this.context.createGain();
        this.silentOutput.gain.value = 0;
        this.microphone.connect(this.capture);
        this.capture.connect(this.silentOutput).connect(this.context.destination);
        this.capture.port.onmessage = (event: MessageEvent<ArrayBuffer>) => this.sendMicrophone(event.data);
      }

      this.onState('connecting');
      this.socket = new WebSocket(
        callAudioWebSocketURL(window.location, this.callID),
        [CALL_AUDIO_PROTOCOL, `sim7600.token.${this.token}`],
      );
      this.socket.binaryType = 'arraybuffer';
      this.socket.onmessage = (event) => this.receive(event.data);
      this.socket.onerror = () => {
        if (!this.disposed) this.fail('Could not connect call audio. Another browser may already be using it.');
      };
      this.socket.onclose = (event) => {
        if (!this.disposed) {
          this.fail(event.reason || 'Call audio disconnected. You can reconnect while the call is active.');
        }
      };
    } catch (error) {
      this.fail(audioErrorMessage(error));
    }
  }

  setMicMuted(muted: boolean) {
    if (this.micMuted !== muted) this.uplinkEnhancer.reset();
    this.micMuted = muted;
  }

  setSpeakerMuted(muted: boolean) {
    this.speakerMuted = muted;
    this.player?.setMuted(muted);
  }

  sendTestTone() {
    if (!this.ready) return;
    this.toneSamplesRemaining = MODEM_SAMPLE_RATE;
    this.tonePhase = 0;
  }

  stop() {
    if (this.disposed) return;
    this.disposed = true;
    this.ready = false;
    this.socket?.close(1000, 'browser audio disconnected');
	this.stopSyntheticClock();
    this.capture?.disconnect();
    this.microphone?.disconnect();
    this.silentOutput?.disconnect();
    this.player?.disconnect();
    this.stream?.getTracks().forEach((track) => track.stop());
    void this.context?.close();
	this.resetLevels();
    this.onState('disconnected');
  }

  private sendMicrophone(inputBuffer: ArrayBuffer) {
    if (!this.ready || this.socket?.readyState !== WebSocket.OPEN || !this.encoder) return;
    // Dropping stale microphone chunks is preferable to accumulating seconds
    // of conversational delay on a congested link.
    if (this.socket.bufferedAmount > 64 * 1024) return;
	let pcm = this.encoder.encode(new Float32Array(inputBuffer));
	if (this.toneSamplesRemaining > 0) {
	  pcm = this.renderTestTone(pcm.length);
	} else if (this.micMuted) {
	  pcm.fill(0);
	} else {
	  pcm = this.uplinkEnhancer.process(pcm);
	}
	this.levels.microphone = pcm16Level(pcm);
	this.publishLevels();
    if (pcm.length > 0) this.socket.send(pcm);
  }

  private receive(data: string | ArrayBuffer | Blob) {
    if (typeof data === 'string') {
      try {
        const message = JSON.parse(data) as { type?: string; sample_rate?: number };
        if (message.type === 'ready' && message.sample_rate === MODEM_SAMPLE_RATE) {
          this.ready = true;
		  if (!this.stream) this.startSyntheticClock();
          this.onState('live');
        }
      } catch {
        this.fail('The call audio service returned an invalid handshake.');
      }
      return;
    }
    if (data instanceof ArrayBuffer) {
	  this.levels.caller = pcm16BufferLevel(data);
	  this.publishLevels();
      this.player?.enqueue(data, MODEM_SAMPLE_RATE);
    }
  }

  private fail(message: string) {
    if (this.disposed) return;
    this.onError(message);
    this.onState('error');
    this.disposed = true;
    this.ready = false;
    this.socket?.close();
	this.stopSyntheticClock();
    this.capture?.disconnect();
    this.microphone?.disconnect();
    this.silentOutput?.disconnect();
    this.player?.disconnect();
    this.stream?.getTracks().forEach((track) => track.stop());
    void this.context?.close();
	this.resetLevels();
  }

  private publishLevels() {
	const now = Date.now();
	if (now - this.lastLevelUpdate < 100) return;
	this.lastLevelUpdate = now;
	this.onLevels({ ...this.levels });
  }

  private resetLevels() {
	this.levels = { microphone: 0, caller: 0 };
	this.onLevels({ ...this.levels });
	this.lastLevelUpdate = 0;
  }

  private startSyntheticClock() {
	if (this.syntheticTimer !== undefined) return;
	this.syntheticTimer = window.setInterval(() => {
	  if (!this.ready || this.socket?.readyState !== WebSocket.OPEN) return;
	  if (this.socket.bufferedAmount > 64 * 1024) return;
	  const pcm = this.toneSamplesRemaining > 0
		? this.renderTestTone(MODEM_SAMPLE_RATE / 50)
		: new Int16Array(MODEM_SAMPLE_RATE / 50);
	  this.levels.microphone = pcm16Level(pcm);
	  this.publishLevels();
	  this.socket.send(pcm);
	}, 20);
  }

  private stopSyntheticClock() {
	if (this.syntheticTimer === undefined) return;
	window.clearInterval(this.syntheticTimer);
	this.syntheticTimer = undefined;
  }

  private renderTestTone(length: number): Int16Array {
	const count = Math.min(length, this.toneSamplesRemaining);
	const rendered = renderSinePCM(count, this.tonePhase);
	const output = new Int16Array(length);
	output.set(rendered.samples);
	this.tonePhase = rendered.nextPhase;
	this.toneSamplesRemaining -= count;
	return output;
  }
}

export function renderSinePCM(
	length: number,
	initialPhase = 0,
	frequency = 440,
	amplitude = 0.15,
): { samples: Int16Array; nextPhase: number } {
	const samples = new Int16Array(length);
	let phase = initialPhase;
	for (let i = 0; i < length; i += 1) {
	  samples[i] = Math.round(Math.sin(phase) * amplitude * 32767);
	  phase += (2 * Math.PI * frequency) / MODEM_SAMPLE_RATE;
	  if (phase >= 2 * Math.PI) phase -= 2 * Math.PI;
	}
	return { samples, nextPhase: phase };
}

export function pcm16Level(samples: Int16Array): number {
	if (samples.length === 0) return 0;
	let sumSquares = 0;
	for (const sample of samples) {
		const normalized = sample / 32768;
		sumSquares += normalized * normalized;
	}
	// Voice typically occupies a small fraction of full scale; expand the
	// visual range while preserving zero as a reliable silence indicator.
	return Math.min(1, Math.sqrt(sumSquares / samples.length) * 4);
}

function pcm16BufferLevel(bytes: ArrayBuffer): number {
	if (bytes.byteLength < 2 || bytes.byteLength % 2 !== 0) return 0;
	const view = new DataView(bytes);
	const samples = new Int16Array(bytes.byteLength / 2);
	for (let i = 0; i < samples.length; i += 1) {
		samples[i] = view.getInt16(i * 2, true);
	}
	return pcm16Level(samples);
}

function audioErrorMessage(error: unknown): string {
	if (error instanceof DOMException) {
		if (error.name === 'NotAllowedError') {
			return 'Microphone permission was denied. Allow microphone access, then try again.';
		}
		if (error.name === 'NotFoundError') {
			return 'No microphone was found. Connect or enable a microphone, then try again.';
		}
		if (error.name === 'NotReadableError') {
			return 'The microphone is busy in another app or could not be opened.';
		}
	}
	return error instanceof Error ? error.message : String(error);
}
