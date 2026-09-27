import "./style.css";
import {Events} from "@wailsio/runtime";
import {SpeechService} from "../bindings/livingspeech";
import * as audio from "./audio";
import * as progress from "./progress";
import {initSettings, refreshSettings} from "./settings";

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
        void SpeechService.SetPanelHeight(Math.ceil(panel.getBoundingClientRect().height));
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
            statusEl.textContent = s.settings.model;
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
        li.textContent = e.text;
        li.title = e.text;
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
        void refreshStatus();
    }
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
initSettings(refreshStatus);

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

async function init() {
    document.getElementById("server-url")!.textContent = await SpeechService.ServerURL();
    screenHeight = (await SpeechService.ScreenHeight()) || screenHeight;
    autosize();
    await Promise.all([refreshRecent(), refreshStatus()]);
}

void init();
