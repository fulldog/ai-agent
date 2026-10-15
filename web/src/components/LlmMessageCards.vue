<template>
  <div v-if="cards.length">
    <p class="legend">
      <span class="tag history">历史消息</span>
      <span class="tag turn">本轮消息</span>
      <span class="tag system">系统提示</span>
    </p>
    <article v-for="(m, i) in cards" :key="i" class="msg" :class="m.span">
      <div class="msg-role">{{ spanLabel(m.span) }} · {{ roleLabel(m.role) }}</div>
      <pre class="msg-body">{{ m.text }}</pre>
    </article>
  </div>
  <pre v-else class="json-block">{{ fallback }}</pre>
</template>

<script setup lang="ts">
import { computed } from "vue";
import type { AgentLLMCall } from "@/api/types";

const props = defineProps<{
  call?: AgentLLMCall;
  input?: string;
  fallback: string;
}>();

type Span = "history" | "turn" | "system";

interface Card {
  role: string;
  span: Span;
  text: string;
}

const cards = computed(() => messageCards(props.call, props.input || ""));

function spanLabel(span: Span): string {
  if (span === "history") return "历史";
  if (span === "turn") return "本轮";
  return "系统";
}

function roleLabel(role: string): string {
  if (role === "user") return "用户";
  if (role === "assistant") return "助手";
  if (role === "system") return "系统";
  if (role === "tool") return "工具";
  return role || "消息";
}

function messageCards(call: AgentLLMCall | undefined, input: string): Card[] {
  const messages = parseMessages(call?.messages);
  if (!messages.length) return [];
  const start = currentTurnStart(messages, input);
  return messages.map((m, i) => ({
    role: m.role,
    span: m.role === "system" ? "system" : i < start ? "history" : "turn",
    text: messageText(m),
  }));
}

interface RawMessage {
  role: string;
  content?: string;
  name?: string;
  tool_calls?: unknown;
}

function parseMessages(raw: unknown): RawMessage[] {
  let value = raw;
  if (typeof value === "string") {
    try {
      value = JSON.parse(value);
    } catch {
      return [];
    }
  }
  if (!Array.isArray(value)) return [];
  return value.map((item) => {
    const row = item && typeof item === "object" ? (item as Record<string, unknown>) : {};
    return {
      role: typeof row.role === "string" ? row.role : "",
      content: typeof row.content === "string" ? row.content : "",
      name: typeof row.name === "string" ? row.name : "",
      tool_calls: row.tool_calls,
    };
  });
}

// 本轮从最后一条用户消息开始。它是这次输入；前面的用户/助手是带入的历史。
function currentTurnStart(messages: RawMessage[], input: string): number {
  const want = input.trim();
  let lastUser = -1;
  let matched = -1;
  messages.forEach((m, i) => {
    if (m.role !== "user") return;
    lastUser = i;
    if (want && (m.content || "").trim() === want) matched = i;
  });
  if (matched >= 0) return matched;
  if (lastUser >= 0) return lastUser;
  return messages.length;
}

function messageText(m: RawMessage): string {
  const parts: string[] = [];
  if (m.name) parts.push(m.name);
  if (m.content) parts.push(m.content);
  if (m.tool_calls != null) {
    try {
      parts.push(JSON.stringify(m.tool_calls, null, 2));
    } catch {
      parts.push(String(m.tool_calls));
    }
  }
  return parts.join("\n\n").trim() || "（空）";
}
</script>

<style scoped>
.legend {
  display: flex;
  gap: 8px;
  margin: 0 0 8px;
}
.tag {
  font-size: 12px;
  line-height: 1;
  padding: 4px 8px;
  border-radius: 4px;
  border: 1px solid transparent;
}
.tag.history,
.msg.history {
  background: #f4f4f5;
  border-color: #dcdfe6;
  color: #606266;
}
.tag.turn,
.msg.turn {
  background: #ecf5ff;
  border-color: #b3d8ff;
  color: #1d4f91;
}
.tag.system,
.msg.system {
  background: #fdf6ec;
  border-color: #f5dab1;
  color: #8a5a12;
}
.msg {
  margin-top: 8px;
  border: 1px solid transparent;
  border-radius: 6px;
  overflow: hidden;
}
.msg-role {
  font-size: 12px;
  font-weight: 600;
  padding: 4px 10px;
}
.msg-body {
  margin: 0;
  padding: 8px 10px;
  background: #fff;
  color: #303133;
  white-space: pre-wrap;
  word-break: break-word;
  font-size: 12px;
  line-height: 1.45;
  max-height: 280px;
  overflow: auto;
}
</style>
