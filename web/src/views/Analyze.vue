<template>
  <div class="page">
    <PageHero :items="hero" />

    <el-row :gutter="12">
      <el-col :span="10">
        <div class="panel">
          <p class="panel-title">上传与参数</p>
          <el-form label-width="96px" @submit.prevent>
            <el-form-item label="文件">
              <el-upload :auto-upload="false" :limit="1" :on-change="onFile">
                <el-button>选择 PDF / DOCX / 图片 / TXT</el-button>
              </el-upload>
            </el-form-item>
            <el-form-item label="说明 / 问题">
              <el-input v-model="message" type="textarea" :rows="3" placeholder="message，与抽取字段至少填一项" />
            </el-form-item>
            <el-form-item label="抽取字段">
              <el-input v-model="fields" placeholder="逗号分隔，如 姓名,金额,日期" />
            </el-form-item>
            <el-form-item label="厂商">
              <el-select v-model="models.selectedProvider" style="width: 140px" @change="models.onProviderChange">
                <el-option v-for="p in models.enabledProviders" :key="p.name" :label="p.name" :value="p.name" />
              </el-select>
            </el-form-item>
            <el-form-item label="模型">
              <el-input v-model="models.selectedModel" style="width: 220px" />
            </el-form-item>
            <el-form-item label="输出">
              <el-checkbox v-model="asJSON">强制 JSON 输出</el-checkbox>
            </el-form-item>
            <el-form-item>
              <el-button type="primary" :loading="loading" @click="run">开始分析</el-button>
            </el-form-item>
          </el-form>
        </div>
      </el-col>
      <el-col :span="14">
        <div class="panel">
          <p class="panel-title">分析结果</p>
          <el-descriptions v-if="meta" :column="2" border size="small" style="margin-bottom: 12px">
            <el-descriptions-item label="文件">{{ meta.file_name || "-" }}</el-descriptions-item>
            <el-descriptions-item label="字符数">{{ meta.file_chars ?? "-" }}</el-descriptions-item>
            <el-descriptions-item label="缓存命中">
              <el-tag size="small" :type="meta.cache_hit ? 'success' : 'info'" effect="light">
                {{ meta.cache_hit ? "是" : "否" }}
              </el-tag>
            </el-descriptions-item>
            <el-descriptions-item label="截断">{{ meta.truncated ? "是" : "否" }}</el-descriptions-item>
            <el-descriptions-item label="抽取后端">{{ meta.extract_backend || "-" }}</el-descriptions-item>
          </el-descriptions>
          <pre v-if="resultJSON" class="json-block">{{ resultJSON }}</pre>
          <p v-else class="muted">选择文件并填写抽取字段后点「开始分析」</p>
        </div>
      </el-col>
    </el-row>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from "vue";
import { ElMessage } from "element-plus";
import type { UploadFile } from "element-plus";
import { Document, Grid, Picture, Refresh } from "@element-plus/icons-vue";
import { requestForm, formatAPIError } from "@/api/client";
import PageHero, { type HeroItem } from "@/components/PageHero.vue";
import { useModelsStore } from "@/stores/models";
import type { AnalyzeResult } from "@/api/types";

const models = useModelsStore();
const file = ref<File | null>(null);
const message = ref("");
const fields = ref("");
const asJSON = ref(true);
const loading = ref(false);
const meta = ref<AnalyzeResult | null>(null);
const resultJSON = ref("");

const hero: HeroItem[] = [
  { icon: Document, title: "文档抽取", desc: "PDF 优先取文字层，缺失时走 OCR", tone: "blue" },
  { icon: Picture, title: "图片 OCR", desc: "依赖本机 Tesseract，需要 chi_sim 语言包", tone: "green" },
  { icon: Grid, title: "结构化字段", desc: "填写字段名后返回对应的 JSON 键值", tone: "purple" },
  { icon: Refresh, title: "抽取缓存", desc: "按文件 SHA256 缓存，重复上传直接命中", tone: "orange" },
];

function onFile(f: UploadFile) {
  file.value = (f.raw as File) || null;
}

async function run() {
  if (!file.value) {
    ElMessage.warning("请选择文件");
    return;
  }
  if (!message.value.trim() && !fields.value.trim()) {
    ElMessage.warning("请填写 message 或抽取字段");
    return;
  }
  const fd = new FormData();
  fd.append("file", file.value);
  if (message.value.trim()) fd.append("message", message.value.trim());
  if (fields.value.trim()) fd.append("fields", fields.value.trim());
  if (models.selectedProvider) fd.append("provider", models.selectedProvider);
  if (models.selectedModel) fd.append("model", models.selectedModel);
  if (asJSON.value) fd.append("response_format", "json");
  loading.value = true;
  try {
    const data = await requestForm<AnalyzeResult>("/api/v1/chat/analyze", fd);
    meta.value = data;
    resultJSON.value = JSON.stringify(data.data ?? data, null, 2);
  } catch (e) {
    ElMessage.error(formatAPIError(e));
  } finally {
    loading.value = false;
  }
}

onMounted(() => {
  models.refresh().catch(() => undefined);
});
</script>
