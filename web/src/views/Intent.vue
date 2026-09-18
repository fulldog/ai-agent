<template>
  <div class="page">
    <PageHero :items="hero" />

    <div class="toolbar">
      <el-form inline @submit.prevent>
        <el-form-item label="文本">
          <el-input v-model="text" clearable placeholder="例如：12345充100" style="width: 320px" />
        </el-form-item>
        <el-form-item label="厂商">
          <el-select v-model="models.selectedProvider" style="width: 140px" @change="models.onProviderChange">
            <el-option v-for="p in models.enabledProviders" :key="p.name" :label="p.name" :value="p.name" />
          </el-select>
        </el-form-item>
        <el-form-item label="模型">
          <el-input v-model="models.selectedModel" style="width: 180px" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="loading" @click="run">解析</el-button>
          <el-button @click="reset">重置</el-button>
        </el-form-item>
      </el-form>
      <div class="toolbar-summary" v-if="meta">{{ meta }}</div>
    </div>

    <div class="table-card">
      <el-table :data="items" v-loading="loading" stripe empty-text="暂无解析结果">
        <el-table-column prop="KeyWordType" label="Type" width="90" />
        <el-table-column label="意图" width="130">
          <template #default="{ row }">
            <el-tag size="small" effect="light">{{ row.KeyWordTypeStr || "-" }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="media_account_id" label="账号" min-width="140" />
        <el-table-column prop="icon_amount" label="金额" width="110" />
        <el-table-column prop="Mobile" label="手机" min-width="130" />
        <el-table-column prop="AuthCode" label="验证码" min-width="110" />
        <el-table-column prop="ForbiddenReason" label="封停原因" min-width="130" />
      </el-table>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from "vue";
import { ElMessage } from "element-plus";
import { Aim, ChatDotSquare, List, Promotion } from "@element-plus/icons-vue";
import { requestJSON, formatAPIError } from "@/api/client";
import PageHero, { type HeroItem } from "@/components/PageHero.vue";
import { useModelsStore } from "@/stores/models";
import type { IntentItem } from "@/api/types";

const models = useModelsStore();
const text = ref("");
const loading = ref(false);
const items = ref<IntentItem[]>([]);
const meta = ref("");

const hero: HeroItem[] = [
  { icon: ChatDotSquare, title: "自然语言输入", desc: "直接粘贴微信助手收到的原始文本", tone: "blue" },
  { icon: Aim, title: "关键字意图", desc: "输出 KeyWordType 与账号、金额等字段", tone: "green" },
  { icon: List, title: "批量结果", desc: "一句话含多个诉求时返回多条记录", tone: "purple" },
  { icon: Promotion, title: "无需数据库", desc: "最小化部署下该接口依然可用", tone: "orange" },
];

async function run() {
  if (!text.value.trim()) return;
  loading.value = true;
  try {
    const data = await requestJSON<{
      data: IntentItem[];
      provider?: string;
      model?: string;
      prompt_tokens?: number;
      completion_tokens?: number;
      msg?: string;
    }>("/api/v1/chat/intent", {
      method: "POST",
      body: JSON.stringify({
        text: text.value.trim(),
        provider: models.selectedProvider || undefined,
        model: models.selectedModel || undefined,
      }),
    });
    items.value = data.data || [];
    meta.value = `${data.provider || ""} ${data.model || ""} · tokens ${data.prompt_tokens ?? "-"}/${data.completion_tokens ?? "-"} ${data.msg || ""}`;
  } catch (e) {
    ElMessage.error(formatAPIError(e));
  } finally {
    loading.value = false;
  }
}

function reset() {
  text.value = "";
  items.value = [];
  meta.value = "";
}

onMounted(() => {
  models.refresh().catch(() => undefined);
});
</script>
