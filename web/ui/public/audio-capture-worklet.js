class SIM7600MicrophoneCapture extends AudioWorkletProcessor {
  constructor() {
    super();
    this.frameSize = Math.max(128, Math.round(sampleRate / 50));
    this.frame = new Float32Array(this.frameSize);
    this.offset = 0;
  }

  process(inputs) {
    const input = inputs[0] && inputs[0][0];
    if (!input) return true;

    let sourceOffset = 0;
    while (sourceOffset < input.length) {
      const count = Math.min(input.length - sourceOffset, this.frame.length - this.offset);
      this.frame.set(input.subarray(sourceOffset, sourceOffset + count), this.offset);
      this.offset += count;
      sourceOffset += count;
      if (this.offset === this.frame.length) {
        const completed = this.frame;
        this.port.postMessage(completed.buffer, [completed.buffer]);
        this.frame = new Float32Array(this.frameSize);
        this.offset = 0;
      }
    }
    return true;
  }
}

registerProcessor('sim7600-microphone-capture', SIM7600MicrophoneCapture);
