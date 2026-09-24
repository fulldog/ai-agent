import { createRouter, createWebHistory } from "vue-router";

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: "/", name: "overview", component: () => import("@/views/Overview.vue"), meta: { title: "概览" } },
    { path: "/chat", name: "chat", component: () => import("@/views/Chat.vue"), meta: { title: "对话", needDB: true } },
    { path: "/conversations", name: "conversations", component: () => import("@/views/Conversations.vue"), meta: { title: "会话历史", needDB: true } },
    { path: "/agent", name: "agent", component: () => import("@/views/Agent.vue"), meta: { title: "Agent", needDB: true } },
    { path: "/agent/runs", name: "agent-runs", component: () => import("@/views/AgentRuns.vue"), meta: { title: "Agent 历史", needDB: true } },
    { path: "/corpus", name: "corpus", component: () => import("@/views/Corpus.vue"), meta: { title: "知识库", needDB: true } },
    { path: "/dingtalk-chats", name: "dingtalk-chats", component: () => import("@/views/DingTalkChats.vue"), meta: { title: "钉钉群", needDB: true } },
    { path: "/rag", name: "rag", component: () => import("@/views/Rag.vue"), meta: { title: "RAG 检索", needDB: true } },
    { path: "/analyze", name: "analyze", component: () => import("@/views/Analyze.vue"), meta: { title: "文件分析" } },
    { path: "/intent", name: "intent", component: () => import("@/views/Intent.vue"), meta: { title: "意图解析" } },
    { path: "/logs", name: "logs", component: () => import("@/views/Logs.vue"), meta: { title: "请求日志", needDB: true } },
    { path: "/settings", name: "settings", component: () => import("@/views/Settings.vue"), meta: { title: "连接设置" } },
  ],
});

export default router;
