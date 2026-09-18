<template>
  <div class="page">
    <PageHero :items="hero" />

    <div class="toolbar">
      <el-form inline @submit.prevent>
        <el-form-item v-if="models.isAdmin" label="UID">
          <el-input v-model="filterUid" clearable placeholder="留空列出全部用户" style="width: 180px" />
        </el-form-item>
        <el-form-item label="状态">
          <el-select v-model="filterStatus" clearable placeholder="全部" style="width: 120px">
            <el-option label="成功" value="succeeded" />
            <el-option label="失败" value="failed" />
            <el-option label="运行中" value="running" />
          </el-select>
        </el-form-item>
        <el-form-item label="输入">
          <el-input v-model="keyword" clearable placeholder="按输入过滤当前页" style="width: 180px" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="loading" @click="reload">查询</el-button>
          <el-button @click="reset">重置</el-button>
        </el-form-item>
      </el-form>
      <div class="toolbar-summary">
        {{ models.isAdmin ? "管理员密钥可查看全部用户" : "仅当前 X-User-Id 的运行" }}
        · 共 {{ total }} 次，当前第 {{ page }} 页
      </div>
    </div>

    <div class="table-card">
      <el-table :data="rows" v-loading="loading" stripe empty-text="暂无运行记录">
        <el-table-column prop="created_at" label="时间" width="180">
          <template #default="{ row }">{{ formatTime(row.created_at) }}</template>
        </el-table-column>
        <el-table-column v-if="models.isAdmin" prop="uid" label="UID" width="120" show-overflow-tooltip />
        <el-table-column label="状态" width="100">
          <template #default="{ row }">
            <span><i class="status-dot" :class="statusTone(row.status)"></i>{{ statusLabel(row.status) }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="model" label="模型" width="160" show-overflow-tooltip />
        <el-table-column label="步数" width="80" prop="step_count" />
        <el-table-column label="输入" min-width="220" show-overflow-tooltip>
          <template #default="{ row }">{{ row.input || "-" }}</template>
        </el-table-column>
        <el-table-column prop="id" label="run_id" min-width="260" show-overflow-tooltip />
        <el-table-column label="操作" width="80" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" @click="open(row)">查看</el-button>
          </template>
        </el-table-column>
      </el-table>
      <div class="pager">
        <span class="pager-total">共 {{ total }} 条</span>
        <el-pagination
          background
          layout="prev, pager, next, jumper"
          :total="total"
          :page-size="limit"
          :current-page="page"
          @current-change="onPage"
        />
      </div>
    </div>

    <el-drawer v-model="drawer" :title="drawerTitle" size="45%" destroy-on-close>
      <template v-if="current">
        <el-descriptions :column="2" border size="small" class="meta">
          <el-descriptions-item label="run_id" :span="2">{{ current.id }}</el-descriptions-item>
          <el-descriptions-item label="时间">{{ formatTime(current.created_at) }}</el-descriptions-item>
          <el-descriptions-item label="状态">
            <span><i class="status-dot" :class="statusTone(current.status)"></i>{{ statusLabel(current.status) }}</span>
          </el-descriptions-item>
          <el-descriptions-item label="模型">{{ current.model || "-" }}</el-descriptions-item>
          <el-descriptions-item label="步数">{{ current.step_count }}</el-descriptions-item>
          <el-descriptions-item label="UID">{{ current.uid || "-" }}</el-descriptions-item>
          <el-descriptions-item label="tokens">{{ current.prompt_tokens }} / {{ current.completion_tokens }}</el-descriptions-item>
          <el-descriptions-item v-if="current.conversation_id" label="会话" :span="2">{{ current.conversation_id }}</el-descriptions-item>
          <el-descriptions-item v-if="current.error_message" label="错误" :span="2">{{ current.error_message }}</el-descriptions-item>
          <el-descriptions-item label="输入" :span="2">{{ current.input || "-" }}</el-descriptions-item>
        </el-descriptions>

        <p class="section-title">执行步骤</p>
        <el-timeline v-if="steps.length">
          <el-timeline-item
            v-for="st in steps"
            :key="st.id || st.step_index"
            :timestamp="stepLabel(st)"
            :type="st.kind === 'tool_result' ? 'primary' : 'success'"
          >
            <pre class="json-block">{{ stepText(st) }}</pre>
          </el-timeline-item>
        </el-timeline>
        <p v-else class="muted">暂无步骤</p>

        <template v-if="current.output">
          <p class="section-title">最终输出</p>
          <div class="md-body" v-html="renderMarkdown(current.output)" />
        </template>
      </template>
    </el-drawer>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { ElMessage } from "element-plus";
import { MagicStick, Search, Timer, View } from "@element-plus/icons-vue";
import { requestJSON, formatAPIError } from "@/api/client";
import { renderMarkdown } from "@/lib/markdown";
import PageHero, { type HeroItem } from "@/components/PageHero.vue";
import { useModelsStore } from "@/stores/models";
import type { AgentRun, AgentStep } from "@/api/types";

const models = useModelsStore();

const hero: HeroItem[] = [
  { icon: Search, title: "运行列表", desc: "admin_api_keys 可查看全部用户，普通密钥仅自己的运行", tone: "blue" },
  { icon: Timer, title: "按状态过滤", desc: "成功 / 失败 / 运行中", tone: "green" },
  { icon: View, title: "查看步骤", desc: "点「查看」在抽屉中回放 tool_call / tool_result", tone: "purple" },
  { icon: MagicStick, title: "关联会话", desc: "绑过会话的旧记录，普通用户也能回溯", tone: "orange" },
];

const filterUid = ref("");
const filterStatus = ref("");
const keyword = ref("");
const allRows = ref<AgentRun[]>([]);
const total = ref(0);
const limit = 20;
const page = ref(1);
const loading = ref(false);
const drawer = ref(false);
const current = ref<AgentRun | null>(null);
const steps = ref<AgentStep[]>([]);

const rows = computed(() => {
  const kw = keyword.value.trim().toLowerCase();
  if (!kw) return allRows.value;
  return allRows.value.filter((r) => (r.input || "").toLowerCase().includes(kw));
});

const drawerTitle = computed(() => {
  if (!current.value) return "运行详情";
  const input = (current.value.input || "未命名任务").slice(0, 24);
  return `${input}${current.value.input && current.value.input.length > 24 ? "…" : ""} · ${statusLabel(current.value.status)}`;
});

function formatTime(v: string): string {
  if (!v) return "-";
  const d = new Date(v);
  return Number.isNaN(d.getTime()) ? v : d.toLocaleString("zh-CN", { hour12: false });
}

function statusTone(s: string): string {
  if (s === "succeeded") return "ok";
  if (s === "failed") return "err";
  return "warn";
}

function statusLabel(s: string): string {
  if (s === "succeeded") return "成功";
  if (s === "failed") return "失败";
  if (s === "running") return "运行中";
  return s || "-";
}

function stepLabel(st: AgentStep): string {
  if (st.kind === "tool_result") return `工具 · ${st.tool_name || "-"}`;
  if (st.kind === "llm") return "LLM";
  return st.kind || "步骤";
}

function stepText(st: AgentStep): string {
  return `${st.tool_name || ""}\n${st.output_text || ""}`.trim() || "-";
}

async function load() {
  loading.value = true;
  const params = new URLSearchParams();
  params.set("limit", String(limit));
  params.set("offset", String((page.value - 1) * limit));
  if (models.isAdmin && filterUid.value.trim()) params.set("uid", filterUid.value.trim());
  if (filterStatus.value) params.set("status", filterStatus.value);
  try {
    const data = await requestJSON<{ items: AgentRun[]; total: number }>(`/api/v1/agent/runs?${params.toString()}`);
    allRows.value = data.items || [];
    total.value = data.total || 0;
  } catch (e) {
    ElMessage.error(formatAPIError(e));
  } finally {
    loading.value = false;
  }
}

function reload() {
  page.value = 1;
  void load();
}

function reset() {
  filterUid.value = "";
  filterStatus.value = "";
  keyword.value = "";
  reload();
}

function onPage(p: number) {
  page.value = p;
  void load();
}

async function open(row: AgentRun) {
  current.value = row;
  steps.value = [];
  drawer.value = true;
  try {
    const data = await requestJSON<{ run: AgentRun; steps: AgentStep[] }>(`/api/v1/agent/runs/${row.id}`);
    current.value = data.run || row;
    steps.value = data.steps || [];
  } catch (e) {
    ElMessage.error(formatAPIError(e));
  }
}

onMounted(() => {
  void load();
});
</script>

<style scoped>
.meta {
  margin-bottom: 8px;
}
.section-title {
  font-size: 14px;
  font-weight: 600;
  margin: 18px 0 10px;
}
</style>
