<template>
  <div class="page">
    <PageHero :items="hero" />

    <div class="toolbar">
      <el-form inline @submit.prevent>
        <el-form-item label="厂商">
          <el-select v-model="models.selectedProvider" style="width: 130px" @change="models.onProviderChange">
            <el-option v-for="p in models.enabledProviders" :key="p.name" :label="p.name" :value="p.name" />
          </el-select>
        </el-form-item>
        <el-form-item label="模型">
          <el-input v-model="models.selectedModel" style="width: 180px" />
        </el-form-item>
        <el-form-item label="会话">
          <el-select v-model="conversationId" clearable filterable placeholder="可选" style="width: 200px">
            <el-option v-for="c in convs" :key="c.id" :label="models.isAdmin && c.uid ? `${c.title || c.id} (${c.uid})` : (c.title || c.id)" :value="c.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="max_steps">
          <el-input-number v-model="maxSteps" :min="1" :max="20" />
        </el-form-item>
        <el-form-item label="语料">
          <el-select v-model="corpusId" clearable filterable placeholder="knowledge_search 用" style="width: 180px">
            <el-option v-for="c in corpora" :key="c.id" :label="c.name" :value="c.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="工具">
          <el-checkbox-group v-model="tools">
            <el-checkbox label="knowledge_search" value="knowledge_search" />
            <el-checkbox label="current_time" value="current_time" />
            <el-checkbox label="calculator" value="calculator" />
          </el-checkbox-group>
        </el-form-item>
      </el-form>
    </div>

    <el-row :gutter="12">
      <el-col :span="10">
        <div class="panel">
          <p class="panel-title">任务输入</p>
          <el-input v-model="input" type="textarea" :rows="5" placeholder="Agent 任务描述" />
          <div style="margin-top: 12px; display: flex; gap: 8px; flex-wrap: wrap">
            <el-button type="primary" :loading="running" @click="run">运行</el-button>
            <el-button v-if="running" @click="stop">停止</el-button>
          </div>
        </div>
      </el-col>
      <el-col :span="14">
        <div class="panel">
          <p class="panel-title">执行时间线</p>
          <el-timeline v-if="events.length">
            <el-timeline-item
              v-for="(ev, i) in events"
              :key="i"
              :timestamp="ev.event"
              :type="ev.event === 'tool_call' ? 'primary' : 'success'"
            >
              <pre class="json-block">{{ ev.text }}</pre>
            </el-timeline-item>
          </el-timeline>
          <p v-else class="muted">暂无事件，运行后展示 tool_call / tool_result</p>

          <template v-if="finalOut">
            <el-divider />
            <p class="panel-title">最终输出</p>
            <div class="md-body" v-html="renderMarkdown(finalOut)" />
          </template>
        </div>
      </el-col>
    </el-row>

    <div class="table-card" style="margin-top: 12px">
      <div class="toolbar" style="margin-bottom: 12px; padding: 0; border: none; background: transparent">
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
            <el-button type="primary" :loading="listLoading" @click="reloadRuns">查询</el-button>
            <el-button @click="resetRuns">重置</el-button>
          </el-form-item>
        </el-form>
        <div class="toolbar-summary">
          {{ models.isAdmin ? "管理员密钥可查看全部用户" : "仅当前 X-User-Id 的运行" }}
          · 共 {{ runTotal }} 次，当前第 {{ runPage }} 页
        </div>
      </div>
      <el-table
        :data="runRows"
        v-loading="listLoading"
        stripe
        highlight-current-row
        empty-text="暂无运行记录"
        @row-click="openRun"
      >
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
            <el-button link type="primary" @click.stop="openRun(row)">查看</el-button>
          </template>
        </el-table-column>
      </el-table>
      <div class="pager">
        <span class="pager-total">共 {{ runTotal }} 条</span>
        <el-pagination
          background
          layout="prev, pager, next, jumper"
          :total="runTotal"
          :page-size="runLimit"
          :current-page="runPage"
          @current-change="onRunPage"
        />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { ElMessage } from "element-plus";
import { Connection, MagicStick, Tools, View } from "@element-plus/icons-vue";
import { requestJSON, formatAPIError } from "@/api/client";
import { postSSE } from "@/api/sse";
import { renderMarkdown } from "@/lib/markdown";
import PageHero, { type HeroItem } from "@/components/PageHero.vue";
import { useModelsStore } from "@/stores/models";
import { useSettingsStore } from "@/stores/settings";
import type { AgentRun, AgentStep, Conversation, Corpus } from "@/api/types";

const models = useModelsStore();
const settings = useSettingsStore();
const input = ref("");
const maxSteps = ref(8);
const tools = ref(["knowledge_search", "current_time", "calculator"]);
const conversationId = ref("");
const corpusId = ref("");
const convs = ref<Conversation[]>([]);
const corpora = ref<Corpus[]>([]);
const running = ref(false);
const events = ref<{ event: string; text: string }[]>([]);
const finalOut = ref("");
const runId = ref("");
const filterUid = ref("");
const filterStatus = ref("");
const keyword = ref("");
const allRuns = ref<AgentRun[]>([]);
const runTotal = ref(0);
const runLimit = 20;
const runPage = ref(1);
const listLoading = ref(false);
let abortCtl: AbortController | null = null;

const hero: HeroItem[] = [
  { icon: MagicStick, title: "工具循环", desc: "模型自行决定调用哪个工具、调用几轮", tone: "blue" },
  { icon: Tools, title: "内置工具", desc: "knowledge_search、current_time、calculator", tone: "green" },
  { icon: Connection, title: "流式事件", desc: "SSE 推送 tool_call / tool_result / delta", tone: "purple" },
  { icon: View, title: "运行回溯", desc: "下方列表可翻看历史运行，点一行加载步骤", tone: "orange" },
];

const runRows = computed(() => {
  const kw = keyword.value.trim().toLowerCase();
  if (!kw) return allRuns.value;
  return allRuns.value.filter((r) => (r.input || "").toLowerCase().includes(kw));
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

async function loadRuns() {
  listLoading.value = true;
  const params = new URLSearchParams();
  params.set("limit", String(runLimit));
  params.set("offset", String((runPage.value - 1) * runLimit));
  if (models.isAdmin && filterUid.value.trim()) params.set("uid", filterUid.value.trim());
  if (filterStatus.value) params.set("status", filterStatus.value);
  try {
    const data = await requestJSON<{ items: AgentRun[]; total: number }>(`/api/v1/agent/runs?${params.toString()}`);
    allRuns.value = data.items || [];
    runTotal.value = data.total || 0;
  } catch (e) {
    ElMessage.error(formatAPIError(e));
  } finally {
    listLoading.value = false;
  }
}

function reloadRuns() {
  runPage.value = 1;
  void loadRuns();
}

function resetRuns() {
  filterUid.value = "";
  filterStatus.value = "";
  keyword.value = "";
  reloadRuns();
}

function onRunPage(p: number) {
  runPage.value = p;
  void loadRuns();
}

function applyRun(run: AgentRun, steps: AgentStep[]) {
  runId.value = run.id;
  events.value = (steps || []).map((s) => ({
    event: s.kind,
    text: `${s.tool_name || ""}\n${s.output_text || ""}`.trim(),
  }));
  finalOut.value = run.output || "";
}

async function openRun(row: AgentRun) {
  try {
    const data = await requestJSON<{ run: AgentRun; steps: AgentStep[] }>(`/api/v1/agent/runs/${row.id}`);
    applyRun(data.run, data.steps || []);
  } catch (e) {
    ElMessage.error(formatAPIError(e));
  }
}

async function run() {
  if (!input.value.trim()) return;
  if (!models.isAdmin && !settings.userId) {
    ElMessage.warning("请先在连接设置填写 X-User-Id");
    return;
  }
  running.value = true;
  events.value = [];
  finalOut.value = "";
  abortCtl = new AbortController();
  const body: Record<string, unknown> = {
    input: input.value.trim(),
    provider: models.selectedProvider || undefined,
    model: models.selectedModel || undefined,
    max_steps: maxSteps.value,
    tools: tools.value,
  };
  if (conversationId.value) body.conversation_id = conversationId.value;
  if (corpusId.value) body.rag = { corpus_id: corpusId.value, top_k: 5 };
  let acc = "";
  try {
    await postSSE(
      "/api/v1/agent/runs/stream",
      body,
      (ev) => {
        if (ev.event === "delta") {
          acc += String(ev.data.content ?? "");
          finalOut.value = acc;
        } else if (ev.event === "tool_call") {
          events.value.push({
            event: "tool_call",
            text: `${ev.data.name || ""} ${ev.data.arguments || ""}`,
          });
        } else if (ev.event === "tool_result") {
          events.value.push({
            event: "tool_result",
            text: `${ev.data.name || ""}\n${ev.data.content || ""}`,
          });
        } else if (ev.event === "done") {
          if (ev.data.output) finalOut.value = String(ev.data.output);
          if (ev.data.run_id) runId.value = String(ev.data.run_id);
        } else if (ev.event === "error") {
          ElMessage.error(String(ev.data.message || "Agent 错误"));
        }
      },
      abortCtl.signal,
    );
  } catch (e) {
    if ((e as Error).name !== "AbortError") ElMessage.error(formatAPIError(e));
  } finally {
    running.value = false;
    abortCtl = null;
    void loadRuns();
  }
}

function stop() {
  abortCtl?.abort();
}

onMounted(async () => {
  await models.refresh().catch(() => undefined);
  try {
    convs.value = (await requestJSON<{ items: Conversation[] }>("/api/v1/conversations")).items || [];
  } catch {
    convs.value = [];
  }
  try {
    corpora.value = (await requestJSON<{ items: Corpus[] }>("/api/v1/corpora")).items || [];
  } catch {
    corpora.value = [];
  }
  void loadRuns();
});
</script>
