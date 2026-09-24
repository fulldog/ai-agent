<template>
  <div class="page">
    <PageHero :items="hero" />

    <el-row :gutter="12">
      <el-col :span="7">
        <div class="panel">
          <div class="panel-head">
            <span class="panel-title" style="margin: 0">语料库</span>
            <el-button type="primary" size="small" @click="createCorpus">新建</el-button>
          </div>
          <el-table :data="corpora" highlight-current-row empty-text="暂无语料库" @current-change="onSelect">
            <el-table-column prop="name" label="名称" min-width="120" show-overflow-tooltip />
            <el-table-column prop="embed_dim" label="维度" width="80" />
            <el-table-column label="操作" width="110">
              <template #default="{ row }">
                <el-button link type="primary" @click.stop="renameCorpus(row)">改名</el-button>
                <el-button link type="danger" @click.stop="delCorpus(row.id)">删除</el-button>
              </template>
            </el-table-column>
          </el-table>
        </div>
      </el-col>
      <el-col :span="17">
        <div v-if="current" class="panel">
          <div class="panel-head">
            <div>
              <span class="panel-title" style="margin: 0">{{ current.name }}</span>
              <div class="muted">{{ current.description || "无描述" }} · {{ current.embed_model }}</div>
            </div>
            <div style="display: flex; gap: 8px">
              <el-button size="small" @click="renameCorpus(current)">修改名称</el-button>
              <el-button size="small" @click="reindex">重建索引</el-button>
            </div>
          </div>

          <el-tabs>
            <el-tab-pane label="上传文件">
              <el-upload
                v-model:file-list="fileList"
                :auto-upload="false"
                multiple
                :on-change="onFileChange"
                :on-remove="onFileRemove"
              >
                <el-button>选择文件（可多选）</el-button>
                <template #tip>
                  <div class="el-upload__tip muted">支持 txt/md/pdf/docx/图片；可一次选择多个文件批量上传索引</div>
                </template>
              </el-upload>
              <el-button type="primary" :loading="uploading" style="margin-top: 8px" @click="uploadFiles">
                上传并索引{{ pendingFiles.length > 1 ? `（${pendingFiles.length} 个）` : "" }}
              </el-button>
            </el-tab-pane>
            <el-tab-pane label="粘贴文本">
              <el-input v-model="docTitle" placeholder="标题" style="margin-bottom: 8px" />
              <el-input v-model="docContent" type="textarea" :rows="8" placeholder="正文" />
              <el-button type="primary" style="margin-top: 8px" @click="addText">添加文档</el-button>
            </el-tab-pane>
          </el-tabs>

          <el-table :data="docs" stripe empty-text="暂无文档" style="margin-top: 12px">
            <el-table-column prop="title" label="标题" min-width="160" show-overflow-tooltip />
            <el-table-column label="状态" width="120">
              <template #default="{ row }">
                <span><i class="status-dot" :class="docTone(row.status)"></i>{{ row.status }}</span>
              </template>
            </el-table-column>
            <el-table-column prop="source" label="来源" min-width="160" show-overflow-tooltip />
            <el-table-column label="操作" width="80" fixed="right">
              <template #default="{ row }">
                <el-button link type="danger" @click="delDoc(row.id)">删除</el-button>
              </template>
            </el-table-column>
          </el-table>
        </div>
        <div v-else class="panel">
          <p class="muted">请选择左侧语料库</p>
        </div>
      </el-col>
    </el-row>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { ElMessage, ElMessageBox } from "element-plus";
import type { UploadFile, UploadUserFile } from "element-plus";
import { Collection, Document, Refresh, Upload } from "@element-plus/icons-vue";
import { requestJSON, requestForm, formatAPIError } from "@/api/client";
import PageHero, { type HeroItem } from "@/components/PageHero.vue";
import type { Corpus, Document as Doc } from "@/api/types";

const hero: HeroItem[] = [
  { icon: Collection, title: "语料库", desc: "按业务划分，名称唯一，绑定 Embedding 模型", tone: "blue" },
  { icon: Upload, title: "批量上传", desc: "支持一次选择多个文件上传并索引", tone: "green" },
  { icon: Document, title: "自动分块", desc: "入库后切分并写入 pgvector 向量列", tone: "purple" },
  { icon: Refresh, title: "重建索引", desc: "更换 Embedding 模型后需要重新索引", tone: "orange" },
];

const corpora = ref<Corpus[]>([]);
const current = ref<Corpus | null>(null);
const docs = ref<Doc[]>([]);
const docTitle = ref("");
const docContent = ref("");
const fileList = ref<UploadUserFile[]>([]);
const uploading = ref(false);

const pendingFiles = computed(() =>
  fileList.value.map((f) => f.raw).filter((f): f is File => !!f),
);

function docTone(status: string): string {
  if (status === "indexed" || status === "ready") return "ok";
  if (status === "failed") return "err";
  return "warn";
}

async function loadCorpora() {
  const data = await requestJSON<{ items: Corpus[] }>("/api/v1/corpora");
  corpora.value = data.items || [];
}

async function loadDocs(id: string) {
  const data = await requestJSON<{ items: Doc[] }>(`/api/v1/corpora/${id}/documents`);
  docs.value = data.items || [];
}

async function onSelect(row: Corpus | null) {
  current.value = row;
  fileList.value = [];
  if (row) await loadDocs(row.id);
  else docs.value = [];
}

async function createCorpus() {
  const { value } = await ElMessageBox.prompt("语料库名称", "新建");
  try {
    await requestJSON("/api/v1/corpora", {
      method: "POST",
      body: JSON.stringify({ name: value, description: "" }),
    });
    await loadCorpora();
  } catch (e) {
    ElMessage.error(formatAPIError(e));
  }
}

async function renameCorpus(row: Corpus) {
  const { value } = await ElMessageBox.prompt("新名称", "修改语料库名称", {
    inputValue: row.name,
    inputPattern: /\S+/,
    inputErrorMessage: "名称不能为空",
  });
  try {
    const updated = await requestJSON<Corpus>(`/api/v1/corpora/${row.id}`, {
      method: "PATCH",
      body: JSON.stringify({ name: value.trim() }),
    });
    await loadCorpora();
    if (current.value?.id === row.id) {
      current.value = { ...current.value, ...updated };
    }
    ElMessage.success("已更新名称");
  } catch (e) {
    ElMessage.error(formatAPIError(e));
  }
}

async function delCorpus(id: string) {
  await ElMessageBox.confirm("删除语料库及其文档？", "确认", { type: "warning" });
  await requestJSON(`/api/v1/corpora/${id}`, { method: "DELETE" });
  current.value = null;
  await loadCorpora();
}

function onFileChange(_f: UploadFile, list: UploadUserFile[]) {
  fileList.value = list;
}

function onFileRemove(_f: UploadFile, list: UploadUserFile[]) {
  fileList.value = list;
}

async function uploadFiles() {
  if (!current.value) return;
  const files = pendingFiles.value;
  if (!files.length) {
    ElMessage.warning("请选择文件");
    return;
  }
  uploading.value = true;
  try {
    const fd = new FormData();
    for (const f of files) {
      fd.append("files", f);
    }
    const res = await requestForm<{
      document?: Doc;
      items?: { filename: string; error?: string }[];
      ok?: number;
      failed?: number;
    }>(`/api/v1/corpora/${current.value.id}/documents`, fd);

    if (res.items && res.items.length > 1) {
      const failed = res.failed ?? res.items.filter((i) => i.error).length;
      const ok = res.ok ?? res.items.length - failed;
      if (failed > 0) {
        const names = res.items.filter((i) => i.error).map((i) => `${i.filename}: ${i.error}`).join("；");
        ElMessage.warning(`成功 ${ok}，失败 ${failed}。${names}`);
      } else {
        ElMessage.success(`已上传 ${ok} 个文件`);
      }
    } else {
      ElMessage.success("已上传");
    }
    fileList.value = [];
    await loadDocs(current.value.id);
  } catch (e) {
    ElMessage.error(formatAPIError(e));
  } finally {
    uploading.value = false;
  }
}

async function addText() {
  if (!current.value || !docContent.value.trim()) return;
  try {
    await requestJSON(`/api/v1/corpora/${current.value.id}/documents`, {
      method: "POST",
      body: JSON.stringify({ title: docTitle.value, content: docContent.value }),
    });
    docTitle.value = "";
    docContent.value = "";
    await loadDocs(current.value.id);
  } catch (e) {
    ElMessage.error(formatAPIError(e));
  }
}

async function delDoc(docId: string) {
  if (!current.value) return;
  await requestJSON(`/api/v1/corpora/${current.value.id}/documents/${docId}`, { method: "DELETE" });
  await loadDocs(current.value.id);
}

async function reindex() {
  if (!current.value) return;
  try {
    await requestJSON(`/api/v1/corpora/${current.value.id}/reindex`, { method: "POST" });
    ElMessage.success("已触发重建索引");
    await loadDocs(current.value.id);
  } catch (e) {
    ElMessage.error(formatAPIError(e));
  }
}

onMounted(async () => {
  try {
    await loadCorpora();
  } catch (e) {
    ElMessage.error(formatAPIError(e));
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
</style>
