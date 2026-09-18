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
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from "vue";
import { ElMessage } from "element-plus";
import { Connection, MagicStick, Tools, View } from "@element-plus/icons-vue";
import { requestJSON, formatAPIError } from "@/api/client";
import { postSSE } from "@/api/sse";
import { renderMarkdown } from "@/lib/markdown";
import PageHero, { type HeroItem } from "@/components/PageHero.vue";
import { useModelsStore } from "@/stores/models";
import { useSettingsStore } from "@/stores/settings";
import type { Conversation, Corpus } from "@/api/types";

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
let abortCtl: AbortController | null = null;

const hero: HeroItem[] = [
  { icon: MagicStick, title: "工具循环", desc: "模型自行决定调用哪个工具、调用几轮", tone: "blue" },
  { icon: Tools, title: "内置工具", desc: "knowledge_search、current_time、calculator", tone: "green" },
  { icon: Connection, title: "流式事件", desc: "SSE 推送 tool_call / tool_result / delta", tone: "purple" },
  { icon: View, title: "运行回溯", desc: "完成后到「Agent 历史」查看每次运行的步骤", tone: "orange" },
];

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
});
</script>
