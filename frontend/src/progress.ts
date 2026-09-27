// A thin two-stage timer for a speech request. The full width is the timeout,
// and the bar grows left to right with the total time spent. It starts in the
// "sending" color (until OpenVox accepts the request) and switches to the
// "waiting" color (until audio arrives), continuing from where it was.

const track = document.getElementById("progress") as HTMLDivElement;
const bar = track.querySelector(".seg") as HTMLDivElement;

let id = 0;
let timeoutMs = 0;
let startedAt = 0;
let frame = 0;

function draw() {
    const pct = Math.min(100, ((performance.now() - startedAt) / timeoutMs) * 100);
    bar.style.width = `${pct}%`;
    frame = requestAnimationFrame(draw);
}

/** Record a stage change reported by the backend. */
export function stage(reqID: number, name: string, timeout: number) {
    if (reqID < id) return;
    if (reqID > id) {
        id = reqID;
        timeoutMs = timeout;
        startedAt = performance.now();
        bar.classList.remove("waiting");
        track.classList.add("active");
        cancelAnimationFrame(frame);
        frame = requestAnimationFrame(draw);
    }
    if (name === "waiting") {
        bar.classList.add("waiting");
    }
}

/** Stop the timer for this request (audio arrived, it failed, or it was cancelled). */
export function finish(reqID: number) {
    if (reqID !== id || !track.classList.contains("active")) return;
    cancelAnimationFrame(frame);
    track.classList.remove("active");
}
