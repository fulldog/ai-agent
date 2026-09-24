<template>
  <el-container class="shell">
    <el-aside width="220px" class="aside">
      <div class="brand">
        <el-icon class="brand-icon"><Cpu /></el-icon>
        <span>AI Agent 控制台</span>
      </div>
      <el-menu :router="true" :default-active="route.path" class="side-menu">
        <el-menu-item index="/">
          <el-icon><Odometer /></el-icon><span>概览</span>
        </el-menu-item>
        <el-sub-menu index="session">
          <template #title>
            <el-icon><ChatDotRound /></el-icon>
            <span>会话</span>
          </template>
          <el-menu-item index="/chat" :disabled="dbOff">对话</el-menu-item>
          <el-menu-item index="/conversations" :disabled="dbOff">会话历史</el-menu-item>
        </el-sub-menu>
        <el-sub-menu index="agent">
          <template #title>
            <el-icon><MagicStick /></el-icon>
            <span>Agent</span>
          </template>
          <el-menu-item index="/agent" :disabled="dbOff">Agent</el-menu-item>
          <el-menu-item index="/agent/runs" :disabled="dbOff">Agent 历史</el-menu-item>
        </el-sub-menu>
        <el-sub-menu index="knowledge">
          <template #title>
            <el-icon><Collection /></el-icon>
            <span>知识库</span>
          </template>
          <el-menu-item index="/corpus" :disabled="dbOff">知识库</el-menu-item>
          <el-menu-item index="/dingtalk-chats" :disabled="dbOff">钉钉群</el-menu-item>
          <el-menu-item index="/rag" :disabled="dbOff">RAG 检索</el-menu-item>
        </el-sub-menu>
        <el-sub-menu index="capability">
          <template #title>
            <el-icon><Aim /></el-icon>
            <span>能力</span>
          </template>
          <el-menu-item index="/analyze">文件分析</el-menu-item>
          <el-menu-item index="/intent">意图解析</el-menu-item>
        </el-sub-menu>
        <el-sub-menu index="system">
          <template #title>
            <el-icon><Setting /></el-icon>
            <span>系统</span>
          </template>
          <el-menu-item index="/logs" :disabled="dbOff">请求日志</el-menu-item>
          <el-menu-item index="/settings">连接设置</el-menu-item>
        </el-sub-menu>
      </el-menu>
    </el-aside>
    <el-container>
      <el-header class="header">
        <div>
          <div class="title">{{ (route.meta.title as string) || "控制台" }}</div>
          <div class="subtitle">{{ subtitle }}</div>
        </div>
        <div class="meta">
          <span class="muted">
            <i class="status-dot" :class="healthDot"></i>{{ healthLabel }}
          </span>
          <span class="muted">UID {{ settings.userId || "未设置" }}</span>
          <el-tag v-if="models.isAdmin" size="small" type="warning" effect="light">管理员</el-tag>
          <span class="muted" v-if="models.selectedProvider">
            {{ models.selectedProvider }} / {{ models.selectedModel }}
          </span>
        </div>
      </el-header>
      <el-main class="main">
        <div v-if="dbOff && route.meta.needDB" class="alert-wrap">
          <el-alert
            title="当前为最小化部署（数据库未启用），本页接口会返回 503。可在「连接设置」确认服务配置。"
            type="warning"
            show-icon
            :closable="false"
          />
        </div>
        <router-view />
      </el-main>
    </el-container>
  </el-container>
</template>

<script setup lang="ts">
import { computed, onMounted } from "vue";
import { useRoute } from "vue-router";
import {
  Aim,
  ChatDotRound,
  Collection,
  Cpu,
  MagicStick,
  Odometer,
  Setting,
} from "@element-plus/icons-vue";
import { useModelsStore } from "@/stores/models";
import { useSettingsStore } from "@/stores/settings";

const route = useRoute();
const models = useModelsStore();
const settings = useSettingsStore();

const subtitles: Record<string, string> = {
  "/": "服务健康、模型厂商与 Token 消耗",
  "/chat": "多轮流式对话，可挂载知识库做 RAG",
  "/conversations": "查询会话；admin_api_keys 可看全部用户",
  "/agent": "工具调用循环，查看每一步的调用与结果",
  "/agent/runs": "回溯历史 Agent 运行，点查看打开步骤抽屉",
  "/corpus": "语料库与文档管理，上传后自动分块索引",
  "/dingtalk-chats": "钉钉群/单聊档案，按群绑定语料库",
  "/rag": "向量检索调试，按相似度查看命中分块",
  "/analyze": "上传 PDF / Word / 图片，抽取结构化字段",
  "/intent": "微信助手关键字意图解析",
  "/logs": "HTTP 与钉钉入站审计日志；详情含关联的 LLM 调用摘要",
  "/settings": "API Key、User Id 与后端地址",
};

const subtitle = computed(() => subtitles[route.path] || "");
const dbOff = computed(() => models.dbDisabled);

const healthLabel = computed(() => {
  if (!models.health) return "未连接";
  const h = models.health;
  if (h.mode === "minimal" || h.db === "disabled") return "最小化部署 / 未连库";
  return `${h.status} · db ${h.db}`;
});

const healthDot = computed(() => {
  if (!models.health) return "";
  if (models.health.status === "ok" && models.health.db === "up") return "ok";
  if (models.health.db === "disabled") return "warn";
  return "err";
});

onMounted(() => {
  models.refresh().catch(() => undefined);
});
</script>

<style scoped>
.shell {
  height: 100%;
}

.aside {
  background: #fff;
  border-right: 1px solid var(--border);
  overflow: auto;
}

.brand {
  height: 56px;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 18px;
  font-weight: 600;
  border-bottom: 1px solid var(--border);
}

.brand-icon {
  color: var(--brand);
  font-size: 18px;
}

.side-menu {
  border-right: none;
}

.side-menu :deep(.el-sub-menu .el-menu-item) {
  min-width: 0;
  padding-left: 48px !important;
}

.header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  background: #fff;
  border-bottom: 1px solid var(--border);
}

.title {
  font-size: 16px;
  font-weight: 600;
}

.subtitle {
  color: var(--text-muted);
  font-size: 12px;
  margin-top: 2px;
}

.meta {
  display: flex;
  gap: 16px;
  align-items: center;
}

.main {
  background: var(--page-bg);
  overflow: auto;
  padding: 0;
}

.alert-wrap {
  padding: 16px 20px 0;
}
</style>
