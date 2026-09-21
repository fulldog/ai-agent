<template>
  <div class="page">
    <PageHero :items="hero" />

    <div class="toolbar">
      <el-form inline @submit.prevent>
        <el-form-item label="语料库">
          <el-select v-model="corpusId" filterable placeholder="选择语料库" style="width: 200px">
            <el-option v-for="c in corpora" :key="c.id" :label="c.name" :value="c.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="查询">
          <el-input v-model="query" clearable placeholder="要检索的问题" style="width: 280px" />
        </el-form-item>
        <el-form-item label="top_k">
          <el-input-number v-model="topK" :min="1" :max="20" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="loading" @click="search">检索</el-button>
          <el-button @click="reset">重置</el-button>
        </el-form-item>
      </el-form>
      <div class="toolbar-summary">
        score 为 cosine 距离，越小越相似；服务端 rag.max_distance
        <template v-if="maxDistance > 0"> = {{ maxDistance }}，超过则丢弃</template>
        <template v-else> 未启用（≤0 不过滤）</template>
      </div>
    </div>

    <div class="table-card">
      <el-table :data="hits" v-loading="loading" stripe empty-text="暂无结果">
        <el-table-column prop="score" label="score" width="120">
          <template #default="{ row }">{{ row.score.toFixed(4) }}</template>
        </el-table-column>
        <el-table-column prop="content" label="命中分块" min-width="320" show-overflow-tooltip />
        <el-table-column prop="document_id" label="document_id" width="280" show-overflow-tooltip />
      </el-table>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from "vue";
import { ElMessage } from "element-plus";
import { Collection, DataAnalysis, Search, Sort } from "@element-plus/icons-vue";
import { requestJSON, formatAPIError } from "@/api/client";
import PageHero, { type HeroItem } from "@/components/PageHero.vue";
import type { Corpus, RagHit } from "@/api/types";

const hero: HeroItem[] = [
  { icon: Collection, title: "选择语料库", desc: "先在知识库页创建语料库并上传文档", tone: "blue" },
  { icon: Search, title: "向量检索", desc: "查询语句会先做 Embedding 再查 pgvector", tone: "green" },
  { icon: Sort, title: "相似度排序", desc: "score 越小越相似；超过 rag.max_distance 的命中会被丢掉", tone: "purple" },
  { icon: DataAnalysis, title: "调参依据", desc: "命中不准时调整分块大小与重叠", tone: "orange" },
];

const corpora = ref<Corpus[]>([]);
const corpusId = ref("");
const query = ref("");
const topK = ref(5);
const hits = ref<RagHit[]>([]);
const maxDistance = ref(0);
const loading = ref(false);

function applyMaxDistance(rows: RagHit[], limit: number): RagHit[] {
  if (!(limit > 0) || !rows.length) {
    return rows;
  }
  return rows.filter((h) => typeof h.score === "number" && h.score <= limit);
}

async function search() {
  if (!corpusId.value || !query.value.trim()) {
    ElMessage.warning("请选择语料库并填写查询");
    return;
  }
  loading.value = true;
  try {
    const data = await requestJSON<{ results: RagHit[]; max_distance?: number }>("/api/v1/rag/search", {
      method: "POST",
      body: JSON.stringify({ corpus_id: corpusId.value, query: query.value.trim(), top_k: topK.value }),
    });
    maxDistance.value = data.max_distance ?? 0;
    hits.value = applyMaxDistance(data.results || [], maxDistance.value);
  } catch (e) {
    ElMessage.error(formatAPIError(e));
  } finally {
    loading.value = false;
  }
}

function reset() {
  query.value = "";
  topK.value = 5;
  hits.value = [];
  maxDistance.value = 0;
}

onMounted(async () => {
  try {
    corpora.value = (await requestJSON<{ items: Corpus[] }>("/api/v1/corpora")).items || [];
  } catch (e) {
    ElMessage.error(formatAPIError(e));
  }
});
</script>
