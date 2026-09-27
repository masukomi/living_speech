import {VoxService} from "../bindings/voxbox";
import type {Option as VoxOption} from "../bindings/voxbox/internal/openvox/models";
import type {Settings} from "../bindings/voxbox/internal/store/models";

const modelSel = document.getElementById("model") as HTMLSelectElement;
const langSel = document.getElementById("language") as HTMLSelectElement;
const voiceSel = document.getElementById("voice") as HTMLSelectElement;
const errorBox = document.getElementById("settings-error") as HTMLDivElement;

// Fill a select and return the chosen value: `preferred` if still offered, else the first option.
function fill(sel: HTMLSelectElement, options: VoxOption[], preferred: string, emptyLabel: string): string {
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
    const voices = (await VoxService.ListVoices(model, language)) ?? [];
    return fill(voiceSel, voices, preferred, "No voices");
}

async function loadLanguages(model: string, preferred: string): Promise<string> {
    let langs: VoxOption[] = [];
    try {
        langs = (await VoxService.ListLanguages(model)) ?? [];
    } catch {
        // Language is optional for some models.
    }
    return fill(langSel, langs, preferred, "Not required");
}

async function save() {
    await VoxService.SaveSettings({
        model: modelSel.value,
        language: langSel.value,
        voice: voiceSel.value,
    } satisfies Settings);
}

/** Fetch fresh lists from OpenVox and select the saved (or default) choices. */
export async function refreshSettings(onChange: () => void): Promise<void> {
    showError("");
    try {
        const saved = await VoxService.GetSettings();
        const models = (await VoxService.ListModels()) ?? [];
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

export function initSettings(onChange: () => void) {
    const guard = (fn: () => Promise<void>) => async () => {
        showError("");
        try {
            await fn();
            onChange();
        } catch (e) {
            showError(String(e));
        }
    };

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
