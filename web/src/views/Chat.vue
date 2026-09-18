<template>
  <div class="chat-page">
    <aside class="conv-list">
      <div class="conv-toolbar">
        <el-button type="primary" size="small" @click="createConv">新建会话</el-button>
        <el-button size="small" @click="loadConvs">刷新</el-button>
      </div>
      <div v-if="models.isAdmin" class="conv-filter">
        <el-input v-model="filterUid" size="small" clearable placeholder="按 UID 筛选" @change="loadConvs" @keyup.enter="loadConvs" />
      </div>
      <el-scrollbar>
        <div
          v-for="c in convs"
          :key="c.id"
          class="conv-item"
          :class="{ active: c.id === currentId }"
          @click="selectConv(c.id)"
        >
          <div class="conv-meta">
            <div class="conv-title">{{ c.title || "未命名" }}</div>
            <div v-if="models.isAdmin" class="conv-uid">{{ c.uid }}</div>
          </div>
          <el-button text type="danger" size="small" @click.stop="removeConv(c.id)">删除</el-button>
        </div>
        <p v-if="!convs.length" class="muted" style="padding: 12px">暂无会话</p>
      </el-scrollbar>
    </aside>
    <section class="chat-main">
      <div class="chat-opts">
        <el-select v-model="models.selectedProvider" placeholder="厂商" style="width: 140px" @change="models.onProviderChange">
          <el-option v-for="p in models.enabledProviders" :key="p.name" :label="p.name" :value="p.name" />
        </el-select>
        <el-input v-model="models.selectedModel" placeholder="模型" style="width: 200px" />
        <el-switch v-model="ragEnabled" active-text="RAG" />
        <el-select v-model="ragCorpus" placeholder="语料库" clearable filterable style="width: 180px" :disabled="!ragEnabled">
          <el-option v-for="c in corpora" :key="c.id" :label="c.name" :value="c.id" />
        </el-select>
        <el-input-number v-model="topK" :min="1" :max="20" :disabled="!ragEnabled" />
      </div>
      <el-scrollbar class="msgs" ref="scrollRef">
        <div v-for="m in messages" :key="m.id" class="msg" :class="m.role">
          <div class="role">{{ m.role }}</div>
          <div class="md-body" v-html="renderMarkdown(m.content)" />
        </div>
      </el-scrollbar>
      <div class="composer">
        <el-input v-model="draft" type="textarea" :rows="3" placeholder="输入消息，Enter 发送（Shift+Enter 换行）" @keydown="onKey" />
        <div class="composer-actions">
          <el-button type="primary" :loading="sending" :disabled="!draft.trim()" @click="send">发送</el-button>
          <el-button v-if="sending" @click="abort">停止</el-button>
          <span v-if="usageText" class="muted">{{ usageText }}</span>
        </div>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { nextTick, onMounted, ref } from "vue";
import { ElMessage, ElMessageBox } from "element-plus";
import { requestJSON, formatAPIError } from "@/api/client";
import { postSSE } from "@/api/sse";
import { renderMarkdown } from "@/lib/markdown";
import { useModelsStore } from "@/stores/models";
import { useSettingsStore } from "@/stores/settings";
import type { Conversation, Corpus, Message } from "@/api/types";

const models = useModelsStore();
const settings = useSettingsStore();
const convs = ref<Conversation[]>([]);
const filterUid = ref("");
const currentId = ref("");
const messages = ref<Message[]>([]);
const draft = ref("");
const sending = ref(false);
const ragEnabled = ref(false);
const ragCorpus = ref("");
const topK = ref(5);
const corpora = ref<Corpus[]>([]);
const usageText = ref("");
const scrollRef = ref();
let abortCtl: AbortController | null = null;

async function loadConvs() {
  try {
    const params = new URLSearchParams();
    params.set("limit", "50");
    if (models.isAdmin && filterUid.value.trim()) params.set("uid", filterUid.value.trim());
    const data = await requestJSON<{ items: Conversation[] }>(`/api/v1/conversations?${params.toString()}`);
    convs.value = data.items || [];
  } catch (e) {
    ElMessage.error(formatAPIError(e));
  }
}

async function loadCorpora() {
  try {
    const data = await requestJSON<{ items: Corpus[] }>("/api/v1/corpora");
    corpora.value = data.items || [];
  } catch {
    corpora.value = [];
  }
}

async function loadMessages(id: string) {
  const data = await requestJSON<{ items: Message[] }>(`/api/v1/conversations/${id}/messages`);
  messages.value = data.items || [];
  await nextTick();
  scrollBottom();
}

function scrollBottom() {
  const wrap = document.querySelector(".msgs .el-scrollbar__wrap");
  if (wrap) wrap.scrollTop = wrap.scrollHeight;
}

async function selectConv(id: string) {
  currentId.value = id;
  usageText.value = "";
  try {
    await loadMessages(id);
  } catch (e) {
    ElMessage.error(formatAPIError(e));
  }
}

async function createConv() {
  try {
    const conv = await requestJSON<Conversation>("/api/v1/conversations", {
      method: "POST",
      body: JSON.stringify({ title: "新会话" }),
    });
    await loadConvs();
    await selectConv(conv.id);
  } catch (e) {
    ElMessage.error(formatAPIError(e));
  }
}

async function removeConv(id: string) {
  await ElMessageBox.confirm("删除该会话？", "确认", { type: "warning" });
  try {
    await requestJSON(`/api/v1/conversations/${id}`, { method: "DELETE" });
    if (currentId.value === id) {
      currentId.value = "";
      messages.value = [];
    }
    await loadConvs();
  } catch (e) {
    if ((e as Error).message?.includes("cancel")) return;
    ElMessage.error(formatAPIError(e));
  }
}

function onKey(e: KeyboardEvent) {
  if (e.key === "Enter" && !e.shiftKey) {
    e.preventDefault();
    void send();
  }
}

async function send() {
  const text = draft.value.trim();
  if (!text || sending.value) return;
  if (!settings.userId) {
    ElMessage.warning("请先在连接设置填写 X-User-Id");
    return;
  }
  sending.value = true;
  abortCtl = new AbortController();
  try {
    if (!currentId.value) {
      const conv = await requestJSON<Conversation>("/api/v1/conversations", {
        method: "POST",
        body: JSON.stringify({ title: text.slice(0, 40) }),
      });
      currentId.value = conv.id;
      await loadConvs();
    }
    messages.value.push({
      id: `tmp-u-${Date.now()}`,
      conversation_id: currentId.value,
      role: "user",
      content: text,
      created_at: new Date().toISOString(),
    });
    const asst: Message = {
      id: `tmp-a-${Date.now()}`,
      conversation_id: currentId.value,
      role: "assistant",
      content: "",
      created_at: new Date().toISOString(),
    };
    messages.value.push(asst);
    draft.value = "";
    usageText.value = "";
    const payload: Record<string, unknown> = {
      conversation_id: currentId.value,
      message: text,
      provider: models.selectedProvider || undefined,
      model: models.selectedModel || undefined,
    };
    if (ragEnabled.value && ragCorpus.value) {
      payload.rag = { enabled: true, corpus_id: ragCorpus.value, top_k: topK.value };
    }
    await postSSE(
      "/api/v1/chat/completions/stream",
      payload,
      (ev) => {
        if (ev.event === "delta") {
          asst.content += String(ev.data.content ?? "");
          void nextTick().then(scrollBottom);
        } else if (ev.event === "done") {
          asst.content = String(ev.data.content ?? asst.content);
          const u = ev.data.usage as { total_tokens?: number } | undefined;
          if (u?.total_tokens != null) usageText.value = `tokens ${u.total_tokens}`;
        } else if (ev.event === "error") {
          ElMessage.error(String(ev.data.message || "流式错误"));
        }
      },
      abortCtl.signal,
    );
    await loadMessages(currentId.value);
  } catch (e) {
    if ((e as Error).name === "AbortError") return;
    ElMessage.error(formatAPIError(e));
  } finally {
    sending.value = false;
    abortCtl = null;
  }
}

function abort() {
  abortCtl?.abort();
}

onMounted(async () => {
  await models.refresh().catch(() => undefined);
  await loadConvs();
  await loadCorpora();
});
</script>

<style scoped>
.chat-page {
  display: flex;
  height: calc(100vh - 88px);
  min-height: 480px;
  margin: 16px 20px 24px;
  background: var(--panel-bg);
  border: 1px solid var(--border);
  border-radius: 4px;
  overflow: hidden;
}
.conv-list {
  width: 240px;
  border-right: 1px solid var(--border);
  display: flex;
  flex-direction: column;
  background: #fafbfc;
}
.conv-toolbar {
  padding: 10px;
  display: flex;
  gap: 8px;
  border-bottom: 1px solid var(--border);
}
.conv-filter {
  padding: 8px 10px;
  border-bottom: 1px solid var(--border);
}
.conv-meta {
  min-width: 0;
  flex: 1;
}
.conv-uid {
  font-size: 12px;
  color: var(--text-muted);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.conv-item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 10px 12px;
  cursor: pointer;
  border-bottom: 1px solid #f0f2f5;
}
.conv-item:hover {
  background: #f0f5ff;
}
.conv-item.active {
  background: #ecf5ff;
  color: var(--brand);
}
.conv-title {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  padding-right: 8px;
}
.chat-main {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-width: 0;
}
.chat-opts {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
  padding: 12px;
  border-bottom: 1px solid var(--border);
  align-items: center;
}
.msgs {
  flex: 1;
  padding: 16px;
}
.msg {
  margin-bottom: 16px;
}
.msg .role {
  font-size: 12px;
  color: var(--text-muted);
  margin-bottom: 4px;
  text-transform: uppercase;
}
.msg.user .md-body {
  background: #ecf5ff;
  padding: 8px 12px;
  border-radius: 6px;
  display: inline-block;
}
.composer {
  border-top: 1px solid var(--border);
  padding: 12px 12px 16px;
}
.composer-actions {
  margin-top: 8px;
  display: flex;
  gap: 8px;
  align-items: center;
}
</style>
