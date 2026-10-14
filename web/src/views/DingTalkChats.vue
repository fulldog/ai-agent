<template>
  <div class="page">
    <PageHero :items="hero" />

    <div class="toolbar">
        <el-form inline @submit.prevent="search">
        <el-form-item label="名称">
          <el-input
            v-model="nameInput"
            clearable
            placeholder="按名称模糊查询"
            style="width: 200px"
            @keyup.enter="search"
          />
        </el-form-item>
        <el-form-item label="类型">
          <el-select v-model="kind" style="width: 140px">
            <el-option label="全部" value="all" />
            <el-option label="群聊" value="group" />
            <el-option label="单聊" value="direct" />
          </el-select>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="search">查询</el-button>
          <el-button :loading="loading" @click="load">刷新</el-button>
        </el-form-item>
      </el-form>
      <div class="toolbar-summary">
        共 {{ filtered.length }} 个会话。每个群可单独设置前置提示词，保存后下一条消息生效。
      </div>
    </div>

    <div class="table-card">
      <el-table :data="filtered" v-loading="loading" stripe :empty-text="emptyText">
        <el-table-column label="类型" width="90">
          <template #default="{ row }">
            <el-tag v-if="row.is_group" size="small" type="warning" effect="light">群聊</el-tag>
            <el-tag v-else size="small" effect="light">单聊</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="title" label="名称" min-width="160" show-overflow-tooltip />
        <el-table-column prop="conversation_id" label="群 ID" min-width="220" show-overflow-tooltip />
        <el-table-column label="数据字典" min-width="280">
          <template #default="{ row }">
            <el-tree-select
              v-model="row.dictionary_document_id"
              :data="dictionaryTree"
              filterable
              clearable
              check-strictly
              :render-after-expand="false"
              placeholder="选择一篇语料"
              no-data-text="暂无语料文档"
              no-match-text="没有匹配的语料"
              popper-class="dict-tree-popper"
              style="width: 100%"
              @change="saveDictionary(row)"
            >
              <template #default="{ node, data }">
                <span
                  class="dict-tree-label"
                  :class="{ 'is-group': !data.title }"
                  @click="onDictionaryLabelClick($event, node, data)"
                >{{ data.title || data.label }}</span>
              </template>
            </el-tree-select>
          </template>
        </el-table-column>
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
              placeholder="选择工具"
              style="width: 100%"
              @change="onToolsChange(row, $event)"
            >
              <el-option
                v-for="t in toolCatalog"
                :key="t.name"
                :label="t.name"
                :value="t.name"
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
      <p class="prompt-hint">这段文字会放在该群系统提示最前面。留空表示不附加群规则。</p>
      <el-input
        v-model="promptDraft"
        type="textarea"
        :rows="16"
        maxlength="4000"
        show-word-limit
        placeholder="留空表示不附加群规则"
      />
      <template #footer>
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
import type { AgentTool, Corpus, DingTalkChat, Document } from "@/api/types";

interface DictionaryGroup {
  id: string;
  name: string;
  documents: { id: string; title: string }[];
}

interface DictionaryTreeNode {
  value: string;
  label: string;
  title?: string;
  disabled?: boolean;
  children?: DictionaryTreeNode[];
}

interface DictionaryTreeState {
  isLeaf: boolean;
  expanded: boolean;
  expand: () => void;
  collapse: () => void;
}

const hero: HeroItem[] = [
  { icon: ChatDotRound, title: "钉钉会话", desc: "按群 ID 记录名称，与发言人会话分开", tone: "blue" },
  { icon: Collection, title: "多库绑定", desc: "一个群可绑多个语料库，只在这些库里检索", tone: "green" },
  { icon: Link, title: "未绑定", desc: "未绑定任何库时仍检索全部语料库", tone: "orange" },
];

const loading = ref(false);
const kind = ref<"all" | "group" | "direct">("all");
const nameInput = ref("");
const nameQuery = ref("");
const rows = ref<DingTalkChat[]>([]);
const corpora = ref<Corpus[]>([]);
const dictionaryGroups = ref<DictionaryGroup[]>([]);
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
  let list = rows.value;
  if (kind.value === "group") list = list.filter((r) => r.is_group);
  else if (kind.value === "direct") list = list.filter((r) => !r.is_group);
  const kw = nameQuery.value.trim().toLowerCase();
  if (!kw) return list;
  return list.filter((r) => (r.title || "").toLowerCase().includes(kw));
});

const emptyText = computed(() =>
  nameQuery.value.trim() ? "没有匹配的会话" : "暂无钉钉会话，请先在群里 @ 机器人",
);

function search() {
  nameQuery.value = nameInput.value.trim();
}

const dictionaryTree = computed<DictionaryTreeNode[]>(() =>
  dictionaryGroups.value.map((g) => ({
    value: `group:${g.id}`,
    label: g.name,
    disabled: true,
    children: g.documents.map((d) => ({
      value: d.id,
      label: `${g.name} / ${d.title}`,
      title: d.title,
    })),
  })),
);

function onDictionaryLabelClick(event: MouseEvent, node: DictionaryTreeState, data: DictionaryTreeNode) {
  if (data.title) return;
  event.stopPropagation();
  const open = (event.currentTarget as HTMLElement).closest(".el-tree-node")?.getAttribute("aria-expanded") === "true";
  if (open) {
    if (!node.expanded) node.expand();
    node.collapse();
    return;
  }
  if (node.expanded) node.collapse();
  node.expand();
}

function noop() {}

function formatTime(v: string): string {
  if (!v) return "-";
  const d = new Date(v);
  return Number.isNaN(d.getTime()) ? v : d.toLocaleString("zh-CN", { hour12: false });
}

function shownTools(row: DingTalkChat): string[] {
  return row.tools || [];
}

function selectedTools(selected: string[]): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  for (const n of selected) {
    if (!n || seen.has(n)) continue;
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
    dictionary_document_id: updated.dictionary_document_id || "",
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
      dictionary_document_id: r.dictionary_document_id || "",
    }));
    corpora.value = corpusRes.items || [];
    toolCatalog.value = toolRes.items || [];
    dictionaryGroups.value = await loadDictionaryGroups(corpora.value);
  } catch (e) {
    ElMessage.error(formatAPIError(e));
  } finally {
    loading.value = false;
  }
}

async function loadDictionaryGroups(items: Corpus[]): Promise<DictionaryGroup[]> {
  const groups = await Promise.all(
    items.map(async (c) => {
      const res = await requestJSON<{ items: Document[] }>(`/api/v1/corpora/${c.id}/documents`);
      const documents = (res.items || [])
        .map((d) => ({
          id: d.id,
          title: d.title?.trim() || d.source?.trim() || "未命名文档",
        }))
        .filter((d) => d.id);
      return { id: c.id, name: c.name, documents };
    }),
  );
  return groups.filter((g) => g.documents.length > 0);
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
  const tools = selectedTools(selected);
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

async function saveDictionary(row: DingTalkChat) {
  const documentID = row.dictionary_document_id || "";
  row.dictionary_document_id = documentID;
  try {
    const updated = await requestJSON<DingTalkChat>(`/api/v1/dingtalk/chats/${row.id}/dictionary`, {
      method: "PUT",
      body: JSON.stringify({ document_id: documentID }),
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

<style>
.dict-tree-popper .el-tree-node__content {
  height: 34px;
}
.dict-tree-popper .el-select-dropdown__item {
  position: relative;
  flex: 1;
  height: 34px;
  line-height: 34px;
  padding: 0;
}
.dict-tree-popper .el-select-dropdown__item.is-disabled {
  color: var(--el-text-color-primary);
  cursor: pointer;
}
.dict-tree-popper .dict-tree-label {
  display: flex;
  align-items: center;
  width: 100%;
  height: 100%;
  padding: 0 8px;
  box-sizing: border-box;
}
.dict-tree-popper .dict-tree-label.is-group {
  font-weight: 600;
}
</style>
