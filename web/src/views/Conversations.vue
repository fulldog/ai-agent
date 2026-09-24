<template>
  <div class="page">
    <PageHero :items="hero" />

    <div class="toolbar">
      <el-form inline @submit.prevent>
        <el-form-item v-if="models.isAdmin" label="UID">
          <el-input v-model="uid" clearable placeholder="留空列出全部用户" style="width: 200px" />
        </el-form-item>
        <el-form-item label="标题">
          <el-input v-model="keyword" clearable placeholder="按标题过滤当前页" style="width: 180px" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="loading" @click="reload">查询</el-button>
          <el-button @click="reset">重置</el-button>
        </el-form-item>
      </el-form>
      <div class="toolbar-summary">
        {{ models.isAdmin ? "管理员密钥可查看全部用户" : "仅当前 X-User-Id 的会话" }}
        · 共 {{ total }} 个会话，当前第 {{ page }} 页
      </div>
    </div>

    <div class="table-card">
      <el-table :data="rows" v-loading="loading" stripe empty-text="暂无会话">
        <el-table-column prop="created_at" label="创建时间" width="180">
          <template #default="{ row }">{{ formatTime(row.created_at) }}</template>
        </el-table-column>
        <el-table-column prop="uid" label="用户 UID" width="150" show-overflow-tooltip />
        <el-table-column label="标题" min-width="180" show-overflow-tooltip>
          <template #default="{ row }">{{ row.title || "未命名" }}</template>
        </el-table-column>
        <el-table-column label="知识库" width="110">
          <template #default="{ row }">
            <el-tag v-if="row.corpus_id" size="small" type="primary" effect="light">已绑定</el-tag>
            <span v-else class="muted">-</span>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="110">
          <template #default="{ row }">
            <span v-if="row.deleted_at"><i class="status-dot err"></i>已删除</span>
            <span v-else><i class="status-dot ok"></i>正常</span>
          </template>
        </el-table-column>
        <el-table-column prop="id" label="会话 ID" min-width="260" show-overflow-tooltip />
        <el-table-column label="操作" width="140" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" @click="open(row)">消息</el-button>
            <el-button v-if="!row.deleted_at" link type="danger" @click="remove(row)">删除</el-button>
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

    <el-drawer v-model="drawer" :title="drawerTitle" size="45%">
      <ChatThread :messages="messages" empty-text="暂无消息" show-time />
    </el-drawer>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { ElMessage, ElMessageBox } from "element-plus";
import { ChatLineSquare, Delete, Search, User } from "@element-plus/icons-vue";
import { requestJSON, formatAPIError } from "@/api/client";
import ChatThread from "@/components/ChatThread.vue";
import PageHero, { type HeroItem } from "@/components/PageHero.vue";
import { useModelsStore } from "@/stores/models";
import type { Conversation, Message } from "@/api/types";

const models = useModelsStore();

const hero: HeroItem[] = [
  { icon: Search, title: "会话列表", desc: "admin_api_keys 可查看全部用户，普通密钥仅自己的会话", tone: "blue" },
  { icon: User, title: "按用户过滤", desc: "管理员密钥可按 UID 筛选指定用户的会话", tone: "green" },
  { icon: ChatLineSquare, title: "查看消息", desc: "点「消息」查看该会话的完整往来记录", tone: "purple" },
  { icon: Delete, title: "删除会话", desc: "软删除后仍在列表，消息可查看，钉钉/控制台不会再往该会话写", tone: "orange" },
];

const uid = ref("");
const keyword = ref("");
const allRows = ref<Conversation[]>([]);
const total = ref(0);
const limit = 20;
const page = ref(1);
const loading = ref(false);
const drawer = ref(false);
const current = ref<Conversation | null>(null);
const messages = ref<Message[]>([]);

const rows = computed(() => {
  const kw = keyword.value.trim().toLowerCase();
  if (!kw) return allRows.value;
  return allRows.value.filter((r) => (r.title || "").toLowerCase().includes(kw));
});

const drawerTitle = computed(() => {
  if (!current.value) return "消息";
  return `${current.value.title || "未命名"} · ${current.value.uid}`;
});

function formatTime(v: string): string {
  if (!v) return "-";
  const d = new Date(v);
  return Number.isNaN(d.getTime()) ? v : d.toLocaleString("zh-CN", { hour12: false });
}

async function load() {
  loading.value = true;
  const params = new URLSearchParams();
  params.set("limit", String(limit));
  params.set("offset", String((page.value - 1) * limit));
  params.set("include_deleted", "1");
  if (models.isAdmin && uid.value.trim()) params.set("uid", uid.value.trim());
  try {
    const data = await requestJSON<{ items: Conversation[]; total: number }>(
      `/api/v1/conversations?${params.toString()}`,
    );
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
  uid.value = "";
  keyword.value = "";
  reload();
}

function onPage(p: number) {
  page.value = p;
  void load();
}

async function open(row: Conversation) {
  current.value = row;
  drawer.value = true;
  try {
    const data = await requestJSON<{ items: Message[] }>(`/api/v1/conversations/${row.id}/messages`);
    messages.value = data.items || [];
  } catch (e) {
    messages.value = [];
    ElMessage.error(formatAPIError(e));
  }
}

async function remove(row: Conversation) {
  await ElMessageBox.confirm(`删除会话「${row.title || row.id}」？`, "确认", { type: "warning" });
  try {
    await requestJSON(`/api/v1/conversations/${row.id}`, { method: "DELETE" });
    ElMessage.success("已删除");
    await load();
  } catch (e) {
    if ((e as Error).message?.includes("cancel")) return;
    ElMessage.error(formatAPIError(e));
  }
}

onMounted(() => {
  void load();
});
</script>
