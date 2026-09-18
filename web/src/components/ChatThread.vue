<template>
  <div class="thread">
    <div v-for="m in messages" :key="m.id" class="row" :class="rowClass(m.role)">
      <div class="bubble" :class="m.role">
        <div class="meta">{{ metaText(m) }}</div>
        <div class="md-body" v-html="renderMarkdown(m.content || '')" />
      </div>
    </div>
    <p v-if="!messages.length && emptyText" class="muted empty">{{ emptyText }}</p>
  </div>
</template>

<script setup lang="ts">
import { renderMarkdown } from "@/lib/markdown";
import type { Message } from "@/api/types";

const props = defineProps<{
  messages: Message[];
  emptyText?: string;
  showTime?: boolean;
}>();

function rowClass(role: string): string {
  return role === "user" ? "right" : "left";
}

function roleLabel(role: string): string {
  switch (role) {
    case "user":
      return "用户";
    case "assistant":
      return "助手";
    case "system":
      return "系统";
    default:
      return role;
  }
}

function formatTime(v: string): string {
  if (!v) return "";
  const d = new Date(v);
  return Number.isNaN(d.getTime()) ? v : d.toLocaleString("zh-CN", { hour12: false });
}

function metaText(m: Message): string {
  const name = roleLabel(m.role);
  if (!props.showTime || !m.created_at) return name;
  return `${name} · ${formatTime(m.created_at)}`;
}
</script>

<style scoped>
.thread {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.row {
  display: flex;
  width: 100%;
}
.row.left {
  justify-content: flex-start;
}
.row.right {
  justify-content: flex-end;
}
.bubble {
  max-width: min(72%, 640px);
  padding: 8px 12px 10px;
  border-radius: 12px;
  word-break: break-word;
}
.row.right .bubble {
  background: #409eff;
  color: #fff;
  border-bottom-right-radius: 4px;
}
.row.left .bubble {
  background: #f4f6f8;
  color: var(--text);
  border: 1px solid var(--border);
  border-bottom-left-radius: 4px;
}
.meta {
  font-size: 12px;
  margin-bottom: 4px;
  opacity: 0.75;
}
.row.right .meta {
  text-align: right;
}
.row.right :deep(.md-body pre),
.row.right :deep(.md-body code) {
  background: rgba(255, 255, 255, 0.16);
  border-color: rgba(255, 255, 255, 0.28);
  color: inherit;
}
.empty {
  margin: 8px 0;
}
</style>
