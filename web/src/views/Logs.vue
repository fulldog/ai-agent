<template>
  <div class="page">
    <PageHero :items="hero" />

    <div class="toolbar">
      <el-form inline @submit.prevent>
        <el-form-item v-if="models.isAdmin" label="UID">
          <el-input v-model="q.uid" clearable placeholder="留空列出全部用户" style="width: 180px" />
        </el-form-item>
        <el-form-item label="路径">
          <el-input v-model="q.path" clearable placeholder="/dingtalk/bot/messages" style="width: 220px" />
        </el-form-item>
        <el-form-item label="request_id">
          <el-input v-model="q.request_id" clearable style="width: 200px" />
        </el-form-item>
        <el-form-item label="会话 ID">
          <el-input v-model="q.conversation_id" clearable style="width: 200px" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="loading" @click="load">查询</el-button>
          <el-button @click="reset">重置</el-button>
        </el-form-item>
      </el-form>
      <div class="toolbar-summary">最近 {{ rows.length }} 条，点击行查看详情</div>
    </div>

    <div class="table-card">
      <el-table :data="rows" v-loading="loading" stripe empty-text="暂无日志" @row-click="open">
        <el-table-column prop="created_at" label="时间" width="180">
          <template #default="{ row }">{{ formatTime(row.created_at) }}</template>
        </el-table-column>
        <el-table-column v-if="models.isAdmin" prop="uid" label="UID" width="120" show-overflow-tooltip />
        <el-table-column label="来源" width="90">
          <template #default="{ row }">
            <el-tag v-if="isDingTalkRow(row)" size="small" type="warning" effect="light">钉钉</el-tag>
            <span v-else>{{ row.method }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="path" label="路径" min-width="220" show-overflow-tooltip />
        <el-table-column label="状态" width="100">
          <template #default="{ row }">
            <span><i class="status-dot" :class="statusTone(row.status)"></i>{{ row.status }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="latency_ms" label="耗时" width="100">
          <template #default="{ row }">{{ row.latency_ms }} ms</template>
        </el-table-column>
        <el-table-column label="流式" width="90">
          <template #default="{ row }">
            <el-tag v-if="row.stream" size="small" effect="light">{{ isDingTalkRow(row) ? "Stream" : "SSE" }}</el-tag>
            <span v-else class="muted">-</span>
          </template>
        </el-table-column>
        <el-table-column prop="request_id" label="request_id" min-width="260" show-overflow-tooltip />
      </el-table>
    </div>

    <el-drawer v-model="drawer" :title="drawerTitle" size="45%" destroy-on-close>
      <template v-if="detail">
        <el-descriptions :column="2" border size="small" class="meta">
          <el-descriptions-item label="request_id" :span="2">{{ detail.request_id }}</el-descriptions-item>
          <el-descriptions-item label="时间">{{ formatTime(detail.created_at) }}</el-descriptions-item>
          <el-descriptions-item label="状态">
            <span><i class="status-dot" :class="statusTone(detail.status)"></i>{{ detail.status }}</span>
          </el-descriptions-item>
          <el-descriptions-item label="耗时">{{ detail.latency_ms }} ms</el-descriptions-item>
          <el-descriptions-item label="UID">{{ detail.uid || "-" }}</el-descriptions-item>
          <el-descriptions-item label="路径" :span="2">{{ detail.method }} {{ detail.path }}</el-descriptions-item>
          <el-descriptions-item v-if="detail.conversation_id" label="会话" :span="2">{{ detail.conversation_id }}</el-descriptions-item>
          <el-descriptions-item v-if="pipeline?.outcome" label="结果">
            <el-tag size="small" :type="outcomeType(pipeline.outcome)" effect="light">{{ outcomeLabel(pipeline.outcome) }}</el-tag>
          </el-descriptions-item>
          <el-descriptions-item v-if="pipeline?.query" label="问题" :span="2">{{ pipeline.query }}</el-descriptions-item>
        </el-descriptions>

        <template v-if="pipeline">
          <p class="section-title">处理步骤</p>
          <div class="steps">
            <article v-for="st in pipeline.steps" :key="`${st.step}-${st.event}`" class="step-card">
              <header class="step-head">
                <span class="step-no" :class="stepTone(st.event)">{{ st.step }}</span>
                <div class="step-head-text">
                  <div class="step-title">{{ stepTitle(st) }}</div>
                  <div class="muted">{{ st.event }}<template v-if="st.at"> · {{ formatTime(st.at) }}</template></div>
                </div>
              </header>
              <div class="step-body">
                <template v-if="st.event === 'dingtalk.receive'">
                  <p v-if="asText(st.detail, 'sender_nick')" class="kv"><span>发送者</span>{{ asText(st.detail, "sender_nick") }}（{{ asText(st.detail, "uid") || "-" }}）</p>
                  <p class="kv"><span>会话</span>{{ asBool(st.detail, "group") ? "群聊" : "单聊" }} · {{ asText(st.detail, "ding_conversation_id") || "-" }}</p>
                  <blockquote class="quote">{{ asText(st.detail, "raw_text") || "（空）" }}</blockquote>
                </template>
                <template v-else-if="st.event === 'dingtalk.rag'">
                  <p class="kv"><span>检索词</span>{{ asText(st.detail, "query") || "-" }}</p>
                  <p class="kv">
                    <span>命中</span>{{ asText(st.detail, "hits") || "0" }} 条
                    <template v-if="asBool(st.detail, 'force_online')"> · 用户要求联网</template>
                  </p>
                  <div v-for="(hit, i) in asHits(st.detail)" :key="i" class="hit">
                    <div class="hit-meta">#{{ i + 1 }} · 距离 {{ formatScore(hit.score) }}</div>
                    <div class="hit-body">{{ hit.content || "（无正文）" }}</div>
                  </div>
                  <p v-if="!asHits(st.detail).length" class="muted">语料库无相关命中</p>
                </template>
                <template v-else-if="st.event === 'dingtalk.llm_request'">
                  <p class="kv"><span>模型</span>{{ asText(st.detail, "provider") }} / {{ asText(st.detail, "model") }}</p>
                  <p class="kv">
                    <span>检索</span>
                    {{ asBool(st.detail, "rag_enabled") ? "使用语料" : "未注入语料" }}
                    <template v-if="asBool(st.detail, 'enable_search')"> · 已开联网搜索</template>
                  </p>
                  <div v-for="(turn, i) in asTurns(st.detail)" :key="i" class="turn" :class="turn.role">
                    <div class="turn-role">{{ roleLabel(turn.role) }}</div>
                    <pre class="turn-body">{{ turn.content }}</pre>
                  </div>
                </template>
                <template v-else-if="st.event === 'dingtalk.result'">
                  <p class="kv"><span>结果</span>{{ outcomeLabel(asText(st.detail, "outcome") || pipeline.outcome || "") }}</p>
                  <p v-if="asText(st.detail, 'elapsed_ms')" class="kv"><span>耗时</span>{{ asText(st.detail, "elapsed_ms") }} ms</p>
                  <p v-if="asText(st.detail, 'error_message')" class="error">{{ asText(st.detail, "error_message") }}</p>
                  <blockquote class="quote result">{{ asText(st.detail, "reply") || detail.response_preview || "（无回复）" }}</blockquote>
                </template>
                <pre v-else class="json-block">{{ prettyJSON(st.detail) }}</pre>
              </div>
            </article>
          </div>
        </template>

        <template v-else>
          <p class="section-title">请求体</p>
          <pre class="json-block">{{ prettyMaybeJSON(detail.request_body) }}</pre>
          <p class="section-title">响应预览</p>
          <pre class="json-block">{{ prettyMaybeJSON(detail.response_preview) }}</pre>
        </template>

        <template v-if="detail.llm_calls?.length">
          <p class="section-title">关联 LLM 调用</p>
          <el-table :data="detail.llm_calls" size="small" stripe>
            <el-table-column prop="provider" label="厂商" width="90" />
            <el-table-column prop="model" label="模型" min-width="140" show-overflow-tooltip />
            <el-table-column prop="status" label="状态" width="80" />
            <el-table-column prop="latency_ms" label="耗时" width="90">
              <template #default="{ row }">{{ row.latency_ms }} ms</template>
            </el-table-column>
            <el-table-column label="tokens" min-width="140">
              <template #default="{ row }">{{ row.prompt_tokens }} / {{ row.completion_tokens }}</template>
            </el-table-column>
          </el-table>
        </template>
      </template>
    </el-drawer>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";
import { ElMessage } from "element-plus";
import { Connection, DataLine, Filter, Timer } from "@element-plus/icons-vue";
import { requestJSON, formatAPIError } from "@/api/client";
import PageHero, { type HeroItem } from "@/components/PageHero.vue";
import { useModelsStore } from "@/stores/models";
import type { RequestLog } from "@/api/types";

interface PipelineStep {
  step: number;
  event: string;
  at?: string;
  detail?: Record<string, unknown>;
}

interface PipelineBody {
  channel?: string;
  request_id?: string;
  query?: string;
  rag_query?: string;
  outcome?: string;
  steps: PipelineStep[];
}

interface RagHitView {
  score?: number;
  content?: string;
}

interface TurnView {
  role: string;
  content: string;
}

const models = useModelsStore();

const hero: HeroItem[] = [
  { icon: Filter, title: "条件筛选", desc: "管理员密钥可按 UID 查看全部请求，普通密钥只看自己的", tone: "blue" },
  { icon: Timer, title: "耗时排查", desc: "记录每个请求的服务端处理毫秒数", tone: "green" },
  { icon: Connection, title: "钉钉四步", desc: "同一 request_id 按收到消息 / 语料检索 / LLM 请求 / 结果展示", tone: "purple" },
  { icon: DataLine, title: "请求详情", desc: "点击任意行查看分步内容，不再堆成一段 JSON", tone: "orange" },
];

const q = reactive({ path: "", request_id: "", conversation_id: "", uid: "" });
const rows = ref<RequestLog[]>([]);
const loading = ref(false);
const drawer = ref(false);
const detail = ref<RequestLog | null>(null);

const pipeline = computed(() => parsePipeline(detail.value?.request_body));
const drawerTitle = computed(() => (pipeline.value ? "钉钉请求详情" : "请求详情"));

function isDingTalkRow(row: RequestLog): boolean {
  return row.method === "STREAM" || (row.path || "").includes("/dingtalk/");
}

function parsePipeline(body?: string): PipelineBody | null {
  if (!body) return null;
  try {
    const v = JSON.parse(body) as PipelineBody;
    if (v && Array.isArray(v.steps) && v.steps.length) return v;
  } catch {
    return null;
  }
  return null;
}

function stepTitle(st: PipelineStep): string {
  switch (st.event) {
    case "dingtalk.receive":
      return "收到钉钉消息";
    case "dingtalk.rag":
      return "语料库检索";
    case "dingtalk.llm_request":
      return "LLM 请求（对话）";
    case "dingtalk.result":
      return "本次请求结果";
    default:
      return `步骤 ${st.step}`;
  }
}

function stepTone(event: string): string {
  switch (event) {
    case "dingtalk.receive":
      return "blue";
    case "dingtalk.rag":
      return "orange";
    case "dingtalk.llm_request":
      return "purple";
    case "dingtalk.result":
      return "green";
    default:
      return "blue";
  }
}

function outcomeLabel(v: string): string {
  switch (v) {
    case "llm":
      return "已调用模型";
    case "corpus_miss":
      return "语料未命中";
    case "rejected":
      return "已拒绝";
    case "error":
      return "失败";
    case "ok":
      return "完成";
    default:
      return v || "-";
  }
}

function outcomeType(v: string): "success" | "warning" | "danger" | "info" {
  if (v === "error") return "danger";
  if (v === "corpus_miss" || v === "rejected") return "warning";
  if (v === "llm" || v === "ok") return "success";
  return "info";
}

function asText(detail: Record<string, unknown> | undefined, key: string): string {
  if (!detail) return "";
  const v = detail[key];
  if (v == null) return "";
  return String(v);
}

function asBool(detail: Record<string, unknown> | undefined, key: string): boolean {
  return Boolean(detail?.[key]);
}

function asHits(detail: Record<string, unknown> | undefined): RagHitView[] {
  const raw = detail?.hits_detail;
  return Array.isArray(raw) ? (raw as RagHitView[]) : [];
}

function asTurns(detail: Record<string, unknown> | undefined): TurnView[] {
  const raw = detail?.conversation;
  if (!Array.isArray(raw)) return [];
  return raw.map((t) => {
    const row = t as Record<string, unknown>;
    return { role: String(row.role || ""), content: String(row.content || "") };
  });
}

function roleLabel(role: string): string {
  if (role === "user") return "用户";
  if (role === "assistant") return "助手";
  if (role === "system") return "系统";
  return role || "消息";
}

function formatScore(v?: number): string {
  if (typeof v !== "number" || Number.isNaN(v)) return "-";
  return v.toFixed(4);
}

function prettyJSON(v: unknown): string {
  try {
    return JSON.stringify(v ?? {}, null, 2);
  } catch {
    return String(v);
  }
}

function prettyMaybeJSON(s?: string): string {
  if (!s) return "（空）";
  try {
    return JSON.stringify(JSON.parse(s), null, 2);
  } catch {
    return s;
  }
}

function formatTime(v: string): string {
  if (!v) return "-";
  const d = new Date(v);
  return Number.isNaN(d.getTime()) ? v : d.toLocaleString("zh-CN", { hour12: false });
}

function statusTone(status: number): string {
  if (status >= 500) return "err";
  if (status >= 400) return "warn";
  return "ok";
}

async function load() {
  loading.value = true;
  const params = new URLSearchParams();
  params.set("limit", "50");
  if (q.path) params.set("path", q.path);
  if (q.request_id) params.set("request_id", q.request_id);
  if (q.conversation_id) params.set("conversation_id", q.conversation_id);
  if (models.isAdmin && q.uid) params.set("uid", q.uid);
  try {
    const data = await requestJSON<{ items: RequestLog[] }>(`/api/v1/logs/requests?${params.toString()}`);
    rows.value = data.items || [];
  } catch (e) {
    ElMessage.error(formatAPIError(e));
  } finally {
    loading.value = false;
  }
}

function reset() {
  q.path = "";
  q.request_id = "";
  q.conversation_id = "";
  q.uid = "";
  void load();
}

async function open(row: RequestLog) {
  try {
    detail.value = await requestJSON<RequestLog>(`/api/v1/logs/requests/${row.id}`);
    drawer.value = true;
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
.steps {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.step-card {
  border: 1px solid var(--border);
  border-radius: 8px;
  background: #fafbfc;
  overflow: hidden;
}
.step-head {
  display: flex;
  gap: 12px;
  align-items: flex-start;
  padding: 12px 14px 10px;
  background: #fff;
  border-bottom: 1px solid var(--border);
}
.step-no {
  width: 28px;
  height: 28px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  font-weight: 700;
  font-size: 13px;
  flex: none;
  color: #fff;
}
.step-no.blue {
  background: #409eff;
}
.step-no.orange {
  background: #e6a23c;
}
.step-no.purple {
  background: #8e5cf7;
}
.step-no.green {
  background: #67c23a;
}
.step-title {
  font-weight: 600;
  line-height: 1.3;
}
.step-body {
  padding: 12px 14px 14px;
}
.kv {
  margin: 0 0 8px;
  line-height: 1.5;
}
.kv span {
  display: inline-block;
  min-width: 52px;
  color: var(--text-muted);
  margin-right: 8px;
}
.quote {
  margin: 8px 0 0;
  padding: 10px 12px;
  background: #fff;
  border-left: 3px solid #409eff;
  border-radius: 0 6px 6px 0;
  white-space: pre-wrap;
  word-break: break-word;
}
.quote.result {
  border-left-color: #67c23a;
}
.hit {
  margin-top: 8px;
  padding: 8px 10px;
  background: #fff;
  border: 1px solid var(--border);
  border-radius: 6px;
}
.hit-meta {
  font-size: 12px;
  color: var(--text-muted);
  margin-bottom: 4px;
}
.hit-body {
  white-space: pre-wrap;
  word-break: break-word;
  line-height: 1.5;
}
.turn {
  margin-top: 8px;
  border: 1px solid var(--border);
  border-radius: 6px;
  overflow: hidden;
  background: #fff;
}
.turn-role {
  font-size: 12px;
  font-weight: 600;
  padding: 4px 10px;
  background: #f0f2f5;
}
.turn.user .turn-role {
  background: #ecf5ff;
  color: #409eff;
}
.turn.system .turn-role {
  background: #fdf6ec;
  color: #e6a23c;
}
.turn-body {
  margin: 0;
  padding: 8px 10px;
  white-space: pre-wrap;
  word-break: break-word;
  font-size: 12px;
  font-family: inherit;
  line-height: 1.5;
}
.error {
  color: #f56c6c;
  margin: 0 0 8px;
}
</style>
