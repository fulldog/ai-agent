<template>
  <div class="page">
    <PageHero :items="hero" />

    <div class="toolbar">
      <el-form inline @submit.prevent>
        <el-form-item v-if="models.isAdmin" label="UID">
          <el-input v-model="q.uid" clearable placeholder="留空列出全部用户" style="width: 180px" />
        </el-form-item>
        <el-form-item label="路径">
          <el-input v-model="q.path" clearable placeholder="/api/v1/chat/completions" style="width: 220px" />
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
        <el-table-column prop="method" label="方法" width="90" />
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
            <el-tag v-if="row.stream" size="small" effect="light">SSE</el-tag>
            <span v-else class="muted">-</span>
          </template>
        </el-table-column>
        <el-table-column prop="request_id" label="request_id" min-width="260" show-overflow-tooltip />
      </el-table>
    </div>

    <el-drawer v-model="drawer" title="请求详情" size="50%">
      <pre v-if="detail" class="json-block">{{ JSON.stringify(detail, null, 2) }}</pre>
    </el-drawer>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from "vue";
import { ElMessage } from "element-plus";
import { Connection, DataLine, Filter, Timer } from "@element-plus/icons-vue";
import { requestJSON, formatAPIError } from "@/api/client";
import PageHero, { type HeroItem } from "@/components/PageHero.vue";
import { useModelsStore } from "@/stores/models";
import type { RequestLog } from "@/api/types";

const models = useModelsStore();

const hero: HeroItem[] = [
  { icon: Filter, title: "条件筛选", desc: "管理员密钥可按 UID 查看全部请求，普通密钥只看自己的", tone: "blue" },
  { icon: Timer, title: "耗时排查", desc: "记录每个请求的服务端处理毫秒数", tone: "green" },
  { icon: Connection, title: "流式标记", desc: "SSE 请求会标注事件数，便于排查中断", tone: "purple" },
  { icon: DataLine, title: "请求详情", desc: "点击任意行查看请求体与响应预览", tone: "orange" },
];

const q = reactive({ path: "", request_id: "", conversation_id: "", uid: "" });
const rows = ref<RequestLog[]>([]);
const loading = ref(false);
const drawer = ref(false);
const detail = ref<RequestLog | null>(null);

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
