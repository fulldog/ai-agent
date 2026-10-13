<template>
  <div class="page">
    <PageHero :items="hero" />

    <div class="toolbar">
      <el-form inline @submit.prevent>
        <el-form-item label="类型">
          <el-select v-model="kind" style="width: 140px">
            <el-option label="全部" value="all" />
            <el-option label="群聊" value="group" />
            <el-option label="单聊" value="direct" />
          </el-select>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="loading" @click="load">刷新</el-button>
        </el-form-item>
      </el-form>
      <div class="toolbar-summary">
        共 {{ filtered.length }} 个会话。每个群可单独设置前置提示词，保存后下一条消息生效。
      </div>
    </div>

    <div class="table-card">
      <el-table :data="filtered" v-loading="loading" stripe empty-text="暂无钉钉会话，请先在群里 @ 机器人">
        <el-table-column label="类型" width="90">
          <template #default="{ row }">
            <el-tag v-if="row.is_group" size="small" type="warning" effect="light">群聊</el-tag>
            <el-tag v-else size="small" effect="light">单聊</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="title" label="名称" min-width="160" show-overflow-tooltip />
        <el-table-column prop="conversation_id" label="群 ID" min-width="220" show-overflow-tooltip />
        <el-table-column label="绑定语料库" min-width="280">
          <template #default="{ row }">
            <el-select
              v-model="row.corpus_ids"
              multiple
              filterable
              clearable
              collapse-tags
              collapse-tags-tooltip
              placeholder="未绑定则检索全部库"
              style="width: 100%"
              @change="save(row)"
            >
              <el-option v-for="c in corpora" :key="c.id" :label="c.name" :value="c.id" />
            </el-select>
          </template>
        </el-table-column>
        <el-table-column label="前置提示词" width="120">
          <template #default="{ row }">
            <el-button link type="primary" @click="openPrompt(row)">
              {{ row.prefix_prompt ? "已设置" : "未设置" }}
            </el-button>
          </template>
        </el-table-column>
        <el-table-column label="工具" min-width="220">
          <template #default="{ row }">
            <el-select
              :model-value="shownTools(row)"
              multiple
              filterable
              collapse-tags
              collapse-tags-tooltip
              placeholder="仅默认工具"
              style="width: 100%"
              @change="onToolsChange(row, $event)"
            >
              <el-option
                v-for="t in toolCatalog"
                :key="t.name"
                :label="t.default ? `${t.name}（始终启用）` : t.name"
                :value="t.name"
                :disabled="!!t.default"
              />
            </el-select>
          </template>
        </el-table-column>
        <el-table-column label="最近消息" width="180">
          <template #default="{ row }">{{ formatTime(row.last_seen_at) }}</template>
        </el-table-column>
      </el-table>
    </div>

    <el-dialog v-model="promptOpen" :title="promptTitle" width="560px" destroy-on-close>
      <p class="prompt-hint">
        这段文字会放在该群系统提示最前面。供应商数据必须同时有名称和标签才能唯一确定一条记录；只给名称时先追问标签，不要查库。
      </p>
      <el-input
        v-model="promptDraft"
        type="textarea"
        :rows="8"
        maxlength="4000"
        show-word-limit
        placeholder="留空表示不附加群规则"
      />
      <template #footer>
        <el-button @click="promptDraft = supplierPrefix">填入供应商规则</el-button>
        <el-button @click="promptOpen = false">取消</el-button>
        <el-button type="primary" :loading="promptSaving" @click="savePrompt">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { ElMessage } from "element-plus";
import { ChatDotRound, Collection, Link } from "@element-plus/icons-vue";
import { requestJSON, formatAPIError } from "@/api/client";
import PageHero, { type HeroItem } from "@/components/PageHero.vue";
import type { AgentTool, Corpus, DingTalkChat } from "@/api/types";

const hero: HeroItem[] = [
  { icon: ChatDotRound, title: "钉钉会话", desc: "按群 ID 记录名称，与发言人会话分开", tone: "blue" },
  { icon: Collection, title: "多库绑定", desc: "一个群可绑多个语料库，只在这些库里检索", tone: "green" },
  { icon: Link, title: "未绑定", desc: "未绑定任何库时仍检索全部语料库", tone: "orange" },
];

const supplierPrefix = `你在回答本群问题时，查询供应商系统必须先唯一确定供应商。

唯一条件：供应商名称和标签缺一不可。同一名称在不同标签下是不同记录，不能只用名称查询，也不能把多个标签的数据混在一起。

标签只允许以下取值。对用户说中文名称，查库时用数字：
- 1 广告线
- 2 本地生活
- 4 数娱线
- 6 电商线
- 8 中后台

缺信息时本轮只追问，不要调用工具，不要查库，不要猜测：
- 只给了名称、没给标签：请用户从上面 5 个标签里选一个。
- 只给了标签、没给名称：请用户补充供应商名称。
- 标签不在上述列表中：列出这 5 个标签，请用户重选。
- 本群历史里已经说过的名称或标签可以沿用，不必重复追问。

名称和标签都明确后，再查当前有效数据，过滤条件必须同时带上该名称和对应标签。回答时说明用的是哪个名称、哪个标签。`;

const loading = ref(false);
const kind = ref<"all" | "group" | "direct">("all");
const rows = ref<DingTalkChat[]>([]);
const corpora = ref<Corpus[]>([]);
const toolCatalog = ref<AgentTool[]>([]);
const promptOpen = ref(false);
const promptSaving = ref(false);
const promptDraft = ref("");
const promptRow = ref<DingTalkChat | null>(null);

const promptTitle = computed(() => {
  const name = promptRow.value?.title || "会话";
  return `前置提示词 · ${name}`;
});

const filtered = computed(() => {
  if (kind.value === "group") return rows.value.filter((r) => r.is_group);
  if (kind.value === "direct") return rows.value.filter((r) => !r.is_group);
  return rows.value;
});

function noop() {}

function formatTime(v: string): string {
  if (!v) return "-";
  const d = new Date(v);
  return Number.isNaN(d.getTime()) ? v : d.toLocaleString("zh-CN", { hour12: false });
}

function defaultToolNames(): string[] {
  return toolCatalog.value.filter((t) => t.default).map((t) => t.name);
}

function shownTools(row: DingTalkChat): string[] {
  const defaults = new Set(defaultToolNames());
  const seen = new Set<string>();
  const out: string[] = [];
  for (const n of row.tools || []) {
    if (!n || defaults.has(n) || seen.has(n)) continue;
    seen.add(n);
    out.push(n);
  }
  return out;
}

function extraTools(selected: string[]): string[] {
  const defaults = new Set(defaultToolNames());
  const seen = new Set<string>();
  const out: string[] = [];
  for (const n of selected) {
    if (!n || defaults.has(n) || seen.has(n)) continue;
    seen.add(n);
    out.push(n);
  }
  return out;
}

function applyChat(updated: DingTalkChat) {
  const i = rows.value.findIndex((r) => r.id === updated.id);
  if (i < 0) return;
  rows.value[i] = {
    ...updated,
    corpus_ids: updated.corpus_ids || [],
    tools: updated.tools || [],
  };
}

async function load() {
  loading.value = true;
  try {
    const [chatRes, corpusRes, toolRes] = await Promise.all([
      requestJSON<{ items: DingTalkChat[] }>("/api/v1/dingtalk/chats"),
      requestJSON<{ items: Corpus[] }>("/api/v1/corpora"),
      requestJSON<{ items: AgentTool[] }>("/api/v1/agent/tools"),
    ]);
    rows.value = (chatRes.items || []).map((r) => ({
      ...r,
      corpus_ids: r.corpus_ids || [],
      tools: r.tools || [],
    }));
    corpora.value = corpusRes.items || [];
    toolCatalog.value = toolRes.items || [];
  } catch (e) {
    ElMessage.error(formatAPIError(e));
  } finally {
    loading.value = false;
  }
}

function openPrompt(row: DingTalkChat) {
  promptRow.value = row;
  promptDraft.value = row.prefix_prompt || "";
  promptOpen.value = true;
}

async function savePrompt() {
  const row = promptRow.value;
  if (!row) return;
  promptSaving.value = true;
  try {
    const updated = await requestJSON<DingTalkChat>(`/api/v1/dingtalk/chats/${row.id}/prefix`, {
      method: "PUT",
      body: JSON.stringify({ prefix_prompt: promptDraft.value }),
    });
    const i = rows.value.findIndex((r) => r.id === row.id);
    if (i >= 0) applyChat(updated);
    promptOpen.value = false;
    ElMessage.success("已保存，下一条消息生效");
  } catch (e) {
    ElMessage.error(formatAPIError(e));
  } finally {
    promptSaving.value = false;
  }
}

function onToolsChange(row: DingTalkChat, value: string[] | string) {
  const selected = Array.isArray(value) ? value : value ? [value] : [];
  void saveTools(row, selected);
}

async function saveTools(row: DingTalkChat, selected: string[]) {
  const tools = extraTools(selected);
  const prev = row.tools || [];
  if (tools.length === prev.length && tools.every((n, i) => n === prev[i])) return;
  try {
    const updated = await requestJSON<DingTalkChat>(`/api/v1/dingtalk/chats/${row.id}/tools`, {
      method: "PUT",
      body: JSON.stringify({ tools }),
    });
    applyChat(updated);
  } catch (e) {
    ElMessage.error(formatAPIError(e));
    await load();
  }
}

async function save(row: DingTalkChat) {
  try {
    const updated = await requestJSON<DingTalkChat>(`/api/v1/dingtalk/chats/${row.id}/corpora`, {
      method: "PUT",
      body: JSON.stringify({ corpus_ids: row.corpus_ids || [] }),
    });
    const i = rows.value.findIndex((r) => r.id === row.id);
    if (i >= 0) applyChat(updated);
  } catch (e) {
    ElMessage.error(formatAPIError(e));
    await load();
  }
}

onMounted(() => {
  void load();
});
</script>

<style scoped>
.prompt-hint {
  margin: 0 0 12px;
  color: var(--el-text-color-secondary);
  line-height: 1.6;
}
</style>
