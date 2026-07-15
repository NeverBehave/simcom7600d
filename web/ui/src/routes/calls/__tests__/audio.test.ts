import { describe, expect, it, vi } from 'vitest';
import {
  BrowserCallAudioSession,
  callAudioWebSocketURL,
  pcm16Level,
  renderSinePCM,
  StreamingPCMEncoder,
  StreamingSpeechEnhancer,
} from '../audio';

describe('StreamingPCMEncoder', () => {
  it('converts a 20 ms 48 kHz browser frame to 160 8 kHz PCM samples', () => {
    const encoder = new StreamingPCMEncoder(48000, 8000);
    const output = encoder.encode(new Float32Array(960).fill(0.5));
    expect(output).toHaveLength(160);
    expect(output[0]).toBe(16384);
    expect(output[159]).toBe(16384);
  });

  it('retains resampling state across browser worklet frames', () => {
    const encoder = new StreamingPCMEncoder(44100, 8000);
    const first = encoder.encode(new Float32Array(441).fill(-0.25));
    const second = encoder.encode(new Float32Array(441).fill(-0.25));
    expect(first.length + second.length).toBe(160);
    expect([...first, ...second].every((sample) => sample === -8192)).toBe(true);
  });

  it('clips browser samples before integer conversion', () => {
    const encoder = new StreamingPCMEncoder(8000, 8000);
    expect([...encoder.encode(Float32Array.from([2, -2]))]).toEqual([32767, -32768]);
  });
});

describe('StreamingSpeechEnhancer', () => {
  it('removes DC and sub-voice rumble from a sustained input', () => {
    const enhancer = new StreamingSpeechEnhancer(8000);
    const output = enhancer.process(new Int16Array(8000).fill(8192));
    expect(rms(output.slice(7000))).toBeLessThan(1);
  });

  it('lifts the consonant band without materially changing lower speech', () => {
    const lowInput = sinePCM(400);
    const highInput = sinePCM(2800);
    const lowOutput = new StreamingSpeechEnhancer(8000).process(lowInput);
    const highOutput = new StreamingSpeechEnhancer(8000).process(highInput);
    const lowGain = rms(lowOutput.slice(1000)) / rms(lowInput.slice(1000));
    const highGain = rms(highOutput.slice(1000)) / rms(highInput.slice(1000));

    expect(lowGain).toBeGreaterThan(0.95);
    expect(lowGain).toBeLessThan(1.15);
    expect(highGain).toBeGreaterThan(1.45);
    expect(highGain).toBeLessThan(1.65);
  });

  it('preserves filter state exactly across transport chunks', () => {
    const input = sinePCM(1200, 0.2, 320);
    const whole = new StreamingSpeechEnhancer(8000).process(input);
    const chunkedEnhancer = new StreamingSpeechEnhancer(8000);
    const chunked = new Int16Array(input.length);
    chunked.set(chunkedEnhancer.process(input.slice(0, 137)), 0);
    chunked.set(chunkedEnhancer.process(input.slice(137)), 137);
    expect(chunked).toEqual(whole);
  });

  it('limits boosted peaks below full scale', () => {
    const input = Int16Array.from({ length: 320 }, (_, index) => index % 2 === 0 ? 32767 : -32768);
    const output = new StreamingSpeechEnhancer(8000).process(input);
    expect(Math.max(...output)).toBeLessThanOrEqual(32113);
    expect(Math.min(...output)).toBeGreaterThanOrEqual(-32113);
  });
});

describe('pcm16Level', () => {
  it('keeps digital silence at zero and scales speech activity', () => {
	expect(pcm16Level(new Int16Array(160))).toBe(0);
	expect(pcm16Level(new Int16Array(160).fill(4096))).toBeCloseTo(0.5, 4);
	expect(pcm16Level(new Int16Array(160).fill(32767))).toBeCloseTo(1, 4);
  });
});

describe('renderSinePCM', () => {
  it('produces a bounded, continuous 440 Hz diagnostic tone', () => {
	const first = renderSinePCM(160);
	const second = renderSinePCM(160, first.nextPhase);
	expect(first.samples).toHaveLength(160);
	expect(first.samples[0]).toBe(0);
	expect(Math.max(...first.samples)).toBeLessThanOrEqual(4916);
	expect(Math.min(...first.samples)).toBeGreaterThanOrEqual(-4916);
	expect(second.samples[0]).not.toBe(0);
  });
});

describe('callAudioWebSocketURL', () => {
  it('uses secure WebSockets with the same host and escapes the call id', () => {
    expect(callAudioWebSocketURL({ protocol: 'https:', host: 'phone.example' } as Location, 'a/b'))
      .toBe('wss://phone.example/v1/calls/a%2Fb/audio');
  });
});

describe('BrowserCallAudioSession', () => {
  it('connects the modem transport when browser audio resume never settles', async () => {
    const mediaDevicesDescriptor = Object.getOwnPropertyDescriptor(navigator, 'mediaDevices');
    Object.defineProperty(navigator, 'mediaDevices', {
      configurable: true,
      value: {
        getUserMedia: vi.fn().mockRejectedValue(new DOMException('missing', 'NotFoundError')),
      },
    });

    class HungAudioContext {
      currentTime = 0;
      destination = {} as AudioDestinationNode;

      resume() {
        return new Promise<AudioContextState>(() => undefined);
      }

      close() {
        return Promise.resolve();
      }

      createGain() {
        return {
          gain: { value: 1, setValueAtTime: vi.fn() },
          connect: vi.fn(),
          disconnect: vi.fn(),
        } as unknown as GainNode;
      }
    }

    class FakeWebSocket {
      static instances: FakeWebSocket[] = [];
      static OPEN = 1;
      binaryType = '';
      bufferedAmount = 0;
      readyState = 0;
      onmessage: ((event: MessageEvent) => void) | null = null;
      onerror: (() => void) | null = null;
      onclose: ((event: CloseEvent) => void) | null = null;

      constructor(public readonly url: string, public readonly protocols: string[]) {
        FakeWebSocket.instances.push(this);
      }

      close() {}
    }

    vi.stubGlobal('AudioContext', HungAudioContext);
    vi.stubGlobal('WebSocket', FakeWebSocket);

    const states: string[] = [];
    const session = new BrowserCallAudioSession(
      'call-1',
      'token-1',
      (state) => states.push(state),
      () => undefined,
    );

    try {
      await expect(Promise.race([
        session.start().then(() => 'started'),
        new Promise<string>((resolve) => window.setTimeout(() => resolve('timed out'), 100)),
      ])).resolves.toBe('started');
      expect(states).toEqual(['requesting', 'connecting']);
      expect(FakeWebSocket.instances).toHaveLength(1);
      expect(FakeWebSocket.instances[0].url).toContain('/v1/calls/call-1/audio');
    } finally {
      session.stop();
      vi.unstubAllGlobals();
      if (mediaDevicesDescriptor) {
        Object.defineProperty(navigator, 'mediaDevices', mediaDevicesDescriptor);
      } else {
        Reflect.deleteProperty(navigator, 'mediaDevices');
      }
    }
  });
});

function sinePCM(frequency: number, amplitude = 0.1, length = 8000): Int16Array {
  return Int16Array.from(
    { length },
    (_, index) => Math.round(Math.sin((2 * Math.PI * frequency * index) / 8000) * amplitude * 32767),
  );
}

function rms(samples: Int16Array): number {
  if (samples.length === 0) return 0;
  let sumSquares = 0;
  for (const sample of samples) sumSquares += sample * sample;
  return Math.sqrt(sumSquares / samples.length);
}
