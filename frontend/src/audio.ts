// Gapless playback of WAV chunks streamed from the backend.

let ctx: AudioContext | null = null;
let nextStart = 0;
let sources: AudioBufferSourceNode[] = [];
let currentID = 0;
// Chunks decode asynchronously; this chain keeps them scheduled in arrival order.
let queue: Promise<void> = Promise.resolve();

function context(): AudioContext {
    if (!ctx) ctx = new AudioContext();
    return ctx;
}

function decodeBase64(b64: string): ArrayBuffer {
    const bin = atob(b64);
    const bytes = new Uint8Array(bin.length);
    for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i);
    return bytes.buffer;
}

/** Start a new utterance, discarding anything still playing. */
export function begin(id: number): void {
    stop();
    currentID = id;
    void context().resume();
}

export function isCurrent(id: number): boolean {
    return id === currentID;
}

export function enqueue(id: number, b64: string, onError: (e: unknown) => void): void {
    queue = queue.then(async () => {
        if (id !== currentID) return;
        const ac = context();
        let buffer: AudioBuffer;
        try {
            buffer = await ac.decodeAudioData(decodeBase64(b64));
        } catch (e) {
            onError(e);
            return;
        }
        if (id !== currentID) return;
        const src = ac.createBufferSource();
        src.buffer = buffer;
        src.connect(ac.destination);
        const at = Math.max(ac.currentTime + 0.02, nextStart);
        src.start(at);
        nextStart = at + buffer.duration;
        sources.push(src);
        src.onended = () => {
            sources = sources.filter((s) => s !== src);
        };
    });
}

/** Resolves once every queued chunk has been scheduled and finished playing. */
export async function finished(id: number): Promise<void> {
    await queue;
    if (id !== currentID || !ctx) return;
    const remaining = nextStart - ctx.currentTime;
    if (remaining > 0) await new Promise((r) => setTimeout(r, remaining * 1000));
}

export function stop(): void {
    currentID = 0;
    for (const s of sources) {
        try { s.stop(); } catch { /* already stopped */ }
    }
    sources = [];
    nextStart = 0;
}
