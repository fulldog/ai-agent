import { defineStore } from "pinia";
import { computed, ref } from "vue";
import { requestJSON } from "@/api/client";
import type { Health, ProviderInfo } from "@/api/types";

export const useModelsStore = defineStore("models", () => {
  const health = ref<Health | null>(null);
  const providers = ref<ProviderInfo[]>([]);
  const loading = ref(false);
  const error = ref("");
  const selectedProvider = ref("");
  const selectedModel = ref("");
  const isAdmin = ref(false);

  const dbReady = computed(() => {
    const db = health.value?.db;
    return db === "up";
  });
  const dbDisabled = computed(() => health.value?.db === "disabled" || health.value?.mode === "minimal");

  const enabledProviders = computed(() => providers.value.filter((p) => p.enabled));

  async function refresh() {
    loading.value = true;
    error.value = "";
    try {
      health.value = await requestJSON<Health>("/health");
    } catch (e) {
      health.value = null;
      error.value = e instanceof Error ? e.message : String(e);
    }
    try {
      const data = await requestJSON<{ providers: ProviderInfo[]; is_admin?: boolean }>("/api/v1/models");
      providers.value = data.providers || [];
      isAdmin.value = Boolean(data.is_admin);
      if (!selectedProvider.value) {
        const first = enabledProviders.value.find((p) => p.configured) || enabledProviders.value[0];
        if (first) {
          selectedProvider.value = first.name;
          selectedModel.value = first.default_model;
        }
      }
    } catch (e) {
      providers.value = [];
      isAdmin.value = false;
      if (!error.value) error.value = e instanceof Error ? e.message : String(e);
    } finally {
      loading.value = false;
    }
  }

  function onProviderChange(name: string) {
    selectedProvider.value = name;
    const p = providers.value.find((x) => x.name === name);
    if (p) selectedModel.value = p.default_model;
  }

  return {
    health,
    providers,
    loading,
    error,
    selectedProvider,
    selectedModel,
    isAdmin,
    dbReady,
    dbDisabled,
    enabledProviders,
    refresh,
    onProviderChange,
  };
});
