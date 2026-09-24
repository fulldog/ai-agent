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
        共 {{ filtered.length }} 个会话。未出现过的群需先 @ 机器人一次；改群名后下次消息会更新。
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
        <el-table-column label="最近消息" width="180">
          <template #default="{ row }">{{ formatTime(row.last_seen_at) }}</template>
        </el-table-column>
      </el-table>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { ElMessage } from "element-plus";
import { ChatDotRound, Collection, Link } from "@element-plus/icons-vue";
import { requestJSON, formatAPIError } from "@/api/client";
import PageHero, { type HeroItem } from "@/components/PageHero.vue";
import type { Corpus, DingTalkChat } from "@/api/types";

const hero: HeroItem[] = [
  { icon: ChatDotRound, title: "钉钉会话", desc: "按群 ID 记录名称，与发言人会话分开", tone: "blue" },
  { icon: Collection, title: "多库绑定", desc: "一个群可绑多个语料库，只在这些库里检索", tone: "green" },
  { icon: Link, title: "未绑定", desc: "未绑定任何库时仍检索全部语料库", tone: "orange" },
];

const loading = ref(false);
const kind = ref<"all" | "group" | "direct">("all");
const rows = ref<DingTalkChat[]>([]);
const corpora = ref<Corpus[]>([]);

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

async function load() {
  loading.value = true;
  try {
    const [chatRes, corpusRes] = await Promise.all([
      requestJSON<{ items: DingTalkChat[] }>("/api/v1/dingtalk/chats"),
      requestJSON<{ items: Corpus[] }>("/api/v1/corpora"),
    ]);
    rows.value = (chatRes.items || []).map((r) => ({
      ...r,
      corpus_ids: r.corpus_ids || [],
    }));
    corpora.value = corpusRes.items || [];
  } catch (e) {
    ElMessage.error(formatAPIError(e));
  } finally {
    loading.value = false;
  }
}

async function save(row: DingTalkChat) {
  try {
    const updated = await requestJSON<DingTalkChat>(`/api/v1/dingtalk/chats/${row.id}/corpora`, {
      method: "PUT",
      body: JSON.stringify({ corpus_ids: row.corpus_ids || [] }),
    });
    const i = rows.value.findIndex((r) => r.id === row.id);
    if (i >= 0) {
      rows.value[i] = { ...updated, corpus_ids: updated.corpus_ids || [] };
    }
  } catch (e) {
    ElMessage.error(formatAPIError(e));
    await load();
  }
}

onMounted(() => {
  void load();
});
</script>
