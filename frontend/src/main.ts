import "./style.css";
import {Events} from "@wailsio/runtime";
import {SpeechService} from "../bindings/livingspeech";
import * as audio from "./audio";
import * as progress from "./progress";
import {DEFAULT_FONT_SIZE, initSettings, refreshSettings} from "./settings";

const panel = document.getElementById("panel") as HTMLDivElement;
const mainView = document.getElementById("main-view") as HTMLElement;
const settingsView = document.getElementById("settings-view") as HTMLElement;
const input = document.getElementById("input") as HTMLTextAreaElement;
const recentList = document.getElementById("recent") as HTMLUListElement;
const statusEl = document.getElementById("status") as HTMLSpanElement;
const banner = document.getElementById("banner") as HTMLDivElement;
const stopBtn = document.getElementById("stop") as HTMLButtonElement;

let screenHeight = window.screen.height;
let activeID = 0;   // newest utterance we're playing
let stoppedUpTo = 0; // ignore audio for utterances at or below this ID

// --- sizing ---

let fitQueued = false;
function fitPanel() {
    if (fitQueued) return;
    fitQueued = true;
    requestAnimationFrame(() => {
        fitQueued = false;
        const rect = panel.getBoundingClientRect();
        void SpeechService.SetPanelSize(Math.ceil(rect.width), Math.ceil(rect.height));
    });
}

// Grow the text box with its content up to half the screen height, then scroll.
function autosize() {
    const maxH = Math.floor(screenHeight * 0.5);
    input.style.height = "auto";
    const needed = input.scrollHeight + 2; // + borders
    input.style.height = `${Math.min(needed, maxH)}px`;
    input.style.overflowY = needed > maxH ? "auto" : "hidden";
    fitPanel();
}

// --- status ---

function showBanner(msg: string) {
    banner.textContent = msg;
    banner.hidden = !msg;
    fitPanel();
}

async function refreshStatus() {
    try {
        const s = await SpeechService.Status();
        if (s.reachable) {
            const avg = s.averageSeconds == null ? "?" : s.averageSeconds.toFixed(1);
            const name = s.settings.engine === "openvox" ? s.settings.model : "System voice";
            statusEl.textContent = name ? `${name} (~${avg} sec.)` : "";
            showBanner("");
        } else {
            statusEl.textContent = "";
            showBanner("OpenVox isn't reachable. Voice output is unavailable right now.");
        }
    } catch (e) {
        showBanner(String(e));
    }
}

// --- recent ---

async function refreshRecent() {
    const entries = (await SpeechService.Recent()) ?? [];
    recentList.replaceChildren();
    if (entries.length === 0) {
        const li = document.createElement("li");
        li.className = "empty";
        li.textContent = "Phrases you speak today show up here.";
        recentList.append(li);
    }
    for (const e of entries) {
        const li = document.createElement("li");
        li.title = e.text;

        // Speak the phrase directly, leaving whatever is in the text box alone.
        const play = document.createElement("button");
        play.className = "icon-btn play";
        play.title = "Speak";
        play.setAttribute("aria-label", `Speak: ${e.text}`);
        // A circled play symbol; a bare triangle reads as an "expand" disclosure arrow.
        play.innerHTML = '<svg viewBox="0 0 24 24"><circle cx="12" cy="12" r="10"/><path d="M10 8.2v7.6l6-3.8z"/></svg>';
        play.addEventListener("click", (ev) => {
            ev.stopPropagation(); // don't also copy it into the text box
            void speak(e.text);
        });

        const text = document.createElement("span");
        text.className = "text";
        text.textContent = e.text;

        li.append(play, text);
        // Clicking the rest of the row puts the phrase in the text box for editing.
        li.addEventListener("click", () => {
            input.value = e.text;
            autosize();
            input.focus();
            input.setSelectionRange(input.value.length, input.value.length);
            input.scrollTop = input.scrollHeight;
        });
        recentList.append(li);
    }
    fitPanel();
}

// --- speech ---

function startPlayback(id: number) {
    if (id <= stoppedUpTo || id < activeID) return;
    if (!audio.isCurrent(id)) {
        activeID = id;
        audio.begin(id);
        stopBtn.hidden = false;
    }
}

async function speak(text: string) {
    showBanner("");
    const id = await SpeechService.Speak(text);
    startPlayback(id);
    void refreshRecent();
}

Events.On("speech:progress", (ev) => {
    const {id, stage, timeoutMs} = ev.data;
    if (id <= stoppedUpTo) return;
    progress.stage(id, stage, timeoutMs);
});

// The system voice plays natively; this marks when its audio began.
Events.On("speech:started", (ev) => {
    progress.finish(ev.data.id);
    startPlayback(ev.data.id);
});

Events.On("speech:chunk", (ev) => {
    const {id, audio: b64} = ev.data;
    progress.finish(id);
    startPlayback(id);
    if (audio.isCurrent(id)) {
        audio.enqueue(id, b64, (e) => showBanner(`Couldn't play audio: ${e}`));
    }
});

Events.On("speech:done", async (ev) => {
    const {id, error} = ev.data;
    progress.finish(id);
    if (id !== activeID) return;
    if (error) {
        showBanner(`Couldn't generate speech. ${error}`);
    }
    void refreshStatus(); // picks up the model's updated average response time
    await audio.finished(id);
    if (id === activeID) stopBtn.hidden = true;
});

stopBtn.addEventListener("click", () => {
    stoppedUpTo = activeID;
    progress.finish(activeID);
    audio.stop();
    stopBtn.hidden = true;
    void SpeechService.StopSpeaking();
});

input.addEventListener("input", autosize);
input.addEventListener("keydown", (e) => {
    if (e.key !== "Enter" || e.shiftKey || e.isComposing) return;
    e.preventDefault();
    const text = input.value.trim();
    if (!text) return;
    input.value = "";
    autosize();
    void speak(text);
});

// --- views ---

function showSettings(show: boolean) {
    mainView.hidden = show;
    settingsView.hidden = !show;
    fitPanel();
    if (show) {
        void refreshSettings(refreshStatus).then(fitPanel);
    } else {
        void refreshStatus();
        input.focus();
    }
}

document.getElementById("open-settings")!.addEventListener("click", () => showSettings(true));
document.getElementById("close-settings")!.addEventListener("click", () => showSettings(false));
document.getElementById("refresh")!.addEventListener("click", () => void refreshSettings(refreshStatus).then(fitPanel));
document.getElementById("preview")!.addEventListener("click", async () => startPlayback(await SpeechService.Preview()));
document.getElementById("quit")!.addEventListener("click", () => void SpeechService.Quit());
initSettings(() => { void refreshStatus(); fitPanel(); }, applyFontSize);

// A double-click on a drag region would trigger macOS's title-bar zoom/minimize.
// The second mousedown precedes the dblclick, so turn dragging off just for it.
document.addEventListener("mousedown", (e) => {
    if (e.detail < 2) return;
    panel.classList.add("no-drag");
    setTimeout(() => panel.classList.remove("no-drag"), 500);
}, true);

// Each time the panel is shown it becomes the key window.
window.addEventListener("focus", () => {
    if (!settingsView.hidden) return;
    input.focus();
    void refreshRecent();
    void refreshStatus();
});

// Everything in the stylesheet is sized in rem, so the root font size scales the whole UI.
function applyFontSize(size: number) {
    document.documentElement.style.fontSize = `${size}px`;
    autosize(); // re-measures the text box and refits the window
}

async function init() {
    try {
        applyFontSize((await SpeechService.GetSettings()).fontSize || DEFAULT_FONT_SIZE);
    } catch {
        // keep the stylesheet default
    }
    document.getElementById("server-url")!.textContent = await SpeechService.ServerURL();
    screenHeight = (await SpeechService.ScreenHeight()) || screenHeight;
    autosize();
    await Promise.all([refreshRecent(), refreshStatus()]);
}

void init();
