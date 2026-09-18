import { defineStore } from "pinia";
import { computed, ref } from "vue";

const KEY = "ai-agent-console-settings";

interface Persisted {
  apiBase: string;
  apiKey: string;
  userId: string;
}

function load(): Persisted {
  try {
    const raw = localStorage.getItem(KEY);
    if (raw) return { apiBase: "", apiKey: "", userId: "", ...JSON.parse(raw) };
  } catch {
    /* ignore */
  }
  return { apiBase: "", apiKey: "", userId: "" };
}

export const useSettingsStore = defineStore("settings", () => {
  const initial = load();
  const apiBase = ref(initial.apiBase);
  const apiKey = ref(initial.apiKey);
  const userId = ref(initial.userId);

  const configured = computed(() => Boolean(apiKey.value.trim()));

  function persist() {
    const data: Persisted = {
      apiBase: apiBase.value.trim(),
      apiKey: apiKey.value.trim(),
      userId: userId.value.trim(),
    };
    localStorage.setItem(KEY, JSON.stringify(data));
  }

  function save(next: Persisted) {
    apiBase.value = next.apiBase.trim();
    apiKey.value = next.apiKey.trim();
    userId.value = next.userId.trim();
    persist();
  }

  return { apiBase, apiKey, userId, configured, save, persist };
});
