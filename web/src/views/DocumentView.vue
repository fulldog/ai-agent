<template>
  <div class="page">
    <div class="panel">
      <div class="panel-head">
        <div>
          <el-button link type="primary" @click="back">返回知识库</el-button>
          <div class="panel-title" style="margin: 8px 0 0">{{ title }}</div>
          <div class="muted">{{ source }}</div>
        </div>
      </div>
      <p v-if="loading" class="muted">加载中…</p>
      <p v-else-if="!content" class="muted">暂无正文</p>
      <div v-else class="md-body doc-body" v-html="html" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { useRoute, useRouter } from "vue-router";
import { ElMessage } from "element-plus";
import { formatAPIError, requestJSON } from "@/api/client";
import { renderMarkdown } from "@/lib/markdown";
import type { Document } from "@/api/types";

const route = useRoute();
const router = useRouter();
const loading = ref(true);
const title = ref("");
const source = ref("");
const content = ref("");

const corpusId = computed(() => String(route.params.corpusId || ""));
const docId = computed(() => String(route.params.docId || ""));
const html = computed(() => renderMarkdown(content.value));

function back() {
  router.push({ path: "/corpus", query: corpusId.value ? { id: corpusId.value } : {} });
}

onMounted(async () => {
  try {
    const data = await requestJSON<{ document: Document; kind: string; content: string }>(
      `/api/v1/corpora/${corpusId.value}/documents/${docId.value}`,
    );
    title.value = data.document?.title || "未命名";
    source.value = data.document?.source || "";
    content.value = data.content || "";
  } catch (e) {
    ElMessage.error(formatAPIError(e));
  } finally {
    loading.value = false;
  }
});
</script>

<style scoped>
.panel-head {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 12px;
  margin-bottom: 12px;
}

.doc-body {
  white-space: normal;
  line-height: 1.7;
}
</style>
