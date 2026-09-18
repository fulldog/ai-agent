<template>
  <div class="page">
    <PageHero :items="hero" />

    <div class="toolbar">
      <el-form inline @submit.prevent>
        <el-form-item>
          <el-button type="primary" :loading="models.loading || usageLoading" @click="reload">刷新</el-button>
        </el-form-item>
      </el-form>
      <div class="toolbar-summary">
        厂商 {{ models.providers.length }} 个，已配置 Key {{ configuredCount }} 个
        <template v-if="usage"> · Token 时区 {{ usage.timezone || "Asia/Shanghai" }}</template>
      </div>
    </div>

    <el-row :gutter="12">
      <el-col :span="8">
        <div class="panel">
          <p class="panel-title">服务健康</p>
          <el-descriptions v-if="models.health" :column="1" border size="small">
            <el-descriptions-item label="status">
              <span><i class="status-dot" :class="models.health.status === 'ok' ? 'ok' : 'err'"></i>{{ models.health.status }}</span>
            </el-descriptions-item>
            <el-descriptions-item label="db">{{ models.health.db }}</el-descriptions-item>
            <el-descriptions-item v-if="models.health.mode" label="mode">{{ models.health.mode }}</el-descriptions-item>
          </el-descriptions>
          <p v-else-if="!models.loading" class="muted">尚未拉取 /health，请先填写 API Key 并刷新</p>
          <el-alert
            v-if="models.error"
            :title="models.error"
            type="error"
            show-icon
            :closable="false"
            style="margin-top: 12px"
          />
        </div>
      </el-col>
      <el-col :span="16">
        <div class="panel">
          <p class="panel-title">已配置 LLM 厂商</p>
          <el-table :data="models.providers" stripe empty-text="无数据">
            <el-table-column prop="name" label="厂商" width="140" />
            <el-table-column prop="default_model" label="默认模型" min-width="160" />
            <el-table-column prop="base_url" label="Base URL" min-width="220" show-overflow-tooltip />
            <el-table-column label="Key" width="100">
              <template #default="{ row }">
                <el-tag size="small" :type="row.configured ? 'success' : 'info'" effect="light">
                  {{ row.configured ? "已配置" : "未配置" }}
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column label="启用" width="100">
              <template #default="{ row }">
                <span><i class="status-dot" :class="row.enabled ? 'ok' : ''"></i>{{ row.enabled ? "启用" : "停用" }}</span>
              </template>
            </el-table-column>
          </el-table>
        </div>
      </el-col>
    </el-row>

    <el-row :gutter="12">
      <el-col v-for="card in usageCards" :key="card.title" :span="6">
        <div class="panel usage-card">
          <div class="muted">{{ card.title }}</div>
          <div class="usage-num">{{ formatTokens(card.bucket.total_tokens) }}</div>
          <div class="muted">输入 {{ formatTokens(card.bucket.prompt_tokens) }} · 输出 {{ formatTokens(card.bucket.completion_tokens) }} · {{ card.bucket.calls }} 次</div>
        </div>
      </el-col>
    </el-row>

    <div class="table-card">
      <p class="panel-title">各模型 Token 消耗</p>
      <p v-if="usageError" class="muted">{{ usageError }}</p>
      <el-table v-else :data="usage?.items || []" v-loading="usageLoading" stripe empty-text="暂无 LLM 调用记录">
        <el-table-column prop="provider" label="厂商" width="120" />
        <el-table-column label="模型" min-width="180" show-overflow-tooltip>
          <template #default="{ row }">{{ row.model || "-" }}</template>
        </el-table-column>
        <el-table-column label="累计" min-width="160">
          <template #default="{ row }">{{ tokenCell(row.all) }}</template>
        </el-table-column>
        <el-table-column label="今日" min-width="140">
          <template #default="{ row }">{{ tokenCell(row.day) }}</template>
        </el-table-column>
        <el-table-column label="本周" min-width="140">
          <template #default="{ row }">{{ tokenCell(row.week) }}</template>
        </el-table-column>
        <el-table-column label="本月" min-width="140">
          <template #default="{ row }">{{ tokenCell(row.month) }}</template>
        </el-table-column>
        <el-table-column label="调用" width="80">
          <template #default="{ row }">{{ row.all.calls }}</template>
        </el-table-column>
      </el-table>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { Coin, Cpu, Monitor, Platform } from "@element-plus/icons-vue";
import { requestJSON, formatAPIError } from "@/api/client";
import PageHero, { type HeroItem } from "@/components/PageHero.vue";
import { useModelsStore } from "@/stores/models";
import type { TokenBucket, TokenUsage } from "@/api/types";

const emptyBucket = (): TokenBucket => ({ calls: 0, prompt_tokens: 0, completion_tokens: 0, total_tokens: 0 });

const models = useModelsStore();
const usage = ref<TokenUsage | null>(null);
const usageLoading = ref(false);
const usageError = ref("");

const hero: HeroItem[] = [
  { icon: Monitor, title: "服务健康", desc: "读取 /health，展示数据库连通性与部署模式", tone: "blue" },
  { icon: Cpu, title: "模型厂商", desc: "列出 YAML 中配置的 OpenAI 兼容上游", tone: "green" },
  { icon: Coin, title: "Token 消耗", desc: "按模型汇总 llm_call_logs 的累计 / 日 / 周 / 月", tone: "purple" },
  { icon: Platform, title: "最小化部署", desc: "未连库时仅文件分析、意图解析可用", tone: "orange" },
];

const configuredCount = computed(() => models.providers.filter((p) => p.configured).length);

const usageCards = computed(() => {
  const t = usage.value?.totals;
  return [
    { title: "累计", bucket: t?.all || emptyBucket() },
    { title: "今日", bucket: t?.day || emptyBucket() },
    { title: "本周（周一至今）", bucket: t?.week || emptyBucket() },
    { title: "本月", bucket: t?.month || emptyBucket() },
  ];
});

function formatTokens(n: number): string {
  return (n || 0).toLocaleString("zh-CN");
}

function tokenCell(b: TokenBucket): string {
  if (!b || !b.total_tokens) return "0";
  return `${formatTokens(b.total_tokens)}（入 ${formatTokens(b.prompt_tokens)} / 出 ${formatTokens(b.completion_tokens)}）`;
}

async function loadUsage() {
  usageLoading.value = true;
  usageError.value = "";
  try {
    usage.value = await requestJSON<TokenUsage>("/api/v1/stats/tokens");
  } catch (e) {
    usage.value = null;
    usageError.value = formatAPIError(e);
  } finally {
    usageLoading.value = false;
  }
}

async function reload() {
  await models.refresh();
  await loadUsage();
}

onMounted(() => {
  void reload();
});
</script>

<style scoped>
.usage-card {
  margin-bottom: 12px;
}
.usage-num {
  font-size: 22px;
  font-weight: 650;
  margin: 6px 0 4px;
}
.table-card .panel-title {
  margin-bottom: 12px;
}
</style>
