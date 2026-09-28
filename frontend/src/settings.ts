import {SpeechService} from "../bindings/livingspeech";
import type {Option as SpeechOption} from "../bindings/livingspeech/internal/openvox/models";
import type {Settings} from "../bindings/livingspeech/internal/store/models";

const engineSel = document.getElementById("engine") as HTMLSelectElement;
const systemFields = document.getElementById("system-fields") as HTMLDivElement;
const openvoxFields = document.getElementById("openvox-fields") as HTMLDivElement;
const refreshBtn = document.getElementById("refresh") as HTMLButtonElement;
const modelSel = document.getElementById("model") as HTMLSelectElement;
const langSel = document.getElementById("language") as HTMLSelectElement;
const voiceSel = document.getElementById("voice") as HTMLSelectElement;
const fontInput = document.getElementById("font-size") as HTMLInputElement;
const errorBox = document.getElementById("settings-error") as HTMLDivElement;

// Fill a select and return the chosen value: `preferred` if still offered, else the first option.
function fill(sel: HTMLSelectElement, options: SpeechOption[], preferred: string, emptyLabel: string): string {
    sel.replaceChildren();
    if (options.length === 0) {
        sel.append(new Option(emptyLabel, ""));
        sel.disabled = true;
        return "";
    }
    sel.disabled = false;
    for (const o of options) {
        sel.append(new Option(o.name || o.id, o.id));
    }
    const chosen = options.some((o) => o.id === preferred) ? preferred : options[0].id;
    sel.value = chosen;
    return chosen;
}

function showError(msg: string) {
    errorBox.textContent = msg;
    errorBox.hidden = !msg;
}

async function loadVoices(model: string, language: string, preferred: string): Promise<string> {
    const voices = (await SpeechService.ListVoices(model, language)) ?? [];
    return fill(voiceSel, voices, preferred, "No voices");
}

async function loadLanguages(model: string, preferred: string): Promise<string> {
    let langs: SpeechOption[] = [];
    try {
        langs = (await SpeechService.ListLanguages(model)) ?? [];
    } catch {
        // Language is optional for some models.
    }
    return fill(langSel, langs, preferred, "Not required");
}

export const DEFAULT_FONT_SIZE = 13;
const MIN_FONT_SIZE = 10;
const MAX_FONT_SIZE = 28;

/** The font size in the input, clamped to a usable range. */
function fontSizeValue(): number {
    const n = Math.round(Number(fontInput.value));
    if (!Number.isFinite(n) || n <= 0) return DEFAULT_FONT_SIZE;
    return Math.min(MAX_FONT_SIZE, Math.max(MIN_FONT_SIZE, n));
}

// The last settings read from disk, so saving with the system voice selected
// doesn't wipe the OpenVox choices (whose lists may never have been loaded).
let saved: Settings | null = null;

function showEngineFields() {
    const openvox = engineSel.value === "openvox";
    openvoxFields.hidden = !openvox;
    systemFields.hidden = openvox;
    refreshBtn.hidden = !openvox;
}

async function save() {
    const listsLoaded = modelSel.options.length > 0 && !modelSel.disabled;
    await SpeechService.SaveSettings({
        engine: engineSel.value,
        model: listsLoaded ? modelSel.value : saved?.model ?? "",
        language: listsLoaded ? langSel.value : saved?.language ?? "",
        voice: listsLoaded ? voiceSel.value : saved?.voice ?? "",
        fontSize: fontInput.value === "" ? 0 : fontSizeValue(), // 0 keeps the saved size
    } satisfies Settings);
}

/** Show the saved settings, fetching fresh lists from OpenVox if it's the engine. */
export async function refreshSettings(onChange: () => void): Promise<void> {
    showError("");
    try {
        saved = await SpeechService.GetSettings();
        fontInput.value = String(saved.fontSize || DEFAULT_FONT_SIZE);
        engineSel.value = saved.engine || "system";
        showEngineFields();
        if (engineSel.value !== "openvox") return;
        const models = (await SpeechService.ListModels()) ?? [];
        const model = fill(modelSel, models, saved.model, "No models");
        if (!model) return;
        const language = await loadLanguages(model, saved.language);
        const voice = await loadVoices(model, language, saved.voice);
        if (model !== saved.model || language !== saved.language || voice !== saved.voice) {
            await save();
            onChange();
        }
    } catch (e) {
        showError(`OpenVox isn't reachable, so the lists couldn't be loaded. ${e ?? ""}`.trim());
    }
}

/**
 * Wire up the settings controls. onChange runs after a voice change is saved;
 * onFontSize runs as the font size is edited, so the UI can resize live.
 */
export function initSettings(onChange: () => void, onFontSize: (size: number) => void) {
    let fontSaveTimer: ReturnType<typeof setTimeout>;
    fontInput.addEventListener("input", () => {
        if (fontInput.value === "") return; // mid-edit
        onFontSize(fontSizeValue());
        clearTimeout(fontSaveTimer);
        fontSaveTimer = setTimeout(() => void save().catch((e) => showError(String(e))), 400);
    });
    fontInput.addEventListener("change", () => {
        fontInput.value = String(fontSizeValue()); // snap out-of-range entries
    });

    const guard = (fn: () => Promise<void>) => async () => {
        showError("");
        try {
            await fn();
            onChange();
        } catch (e) {
            showError(String(e));
        }
    };

    engineSel.addEventListener("change", guard(async () => {
        showEngineFields();
        await save();
        if (engineSel.value === "openvox") {
            await refreshSettings(onChange); // load the model, language, and voice lists
        }
    }));
    document.getElementById("open-spoken-content")!.addEventListener("click",
        () => void SpeechService.OpenSpokenContentSettings().catch((e) => showError(String(e))));

    modelSel.addEventListener("change", guard(async () => {
        const language = await loadLanguages(modelSel.value, langSel.value);
        await loadVoices(modelSel.value, language, voiceSel.value);
        await save();
    }));
    langSel.addEventListener("change", guard(async () => {
        await loadVoices(modelSel.value, langSel.value, voiceSel.value);
        await save();
    }));
    voiceSel.addEventListener("change", guard(save));
}
