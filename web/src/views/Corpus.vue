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
          <el-table
            :data="corpora"
            row-key="id"
            highlight-current-row
            :current-row-key="current?.id"
            empty-text="暂无语料库"
            @current-change="onSelect"
          >
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
                  <div class="el-upload__tip muted">支持 txt/md/pdf/docx/图片。一次可选多个文件，每个文件单独建索引，可单独删除或重传</div>
                </template>
              </el-upload>
              <el-button type="primary" :loading="uploading" style="margin-top: 8px" @click="uploadFiles">
                上传并索引{{ pendingFiles.length > 1 ? `（${pendingFiles.length} 个，分别建索引）` : "" }}
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
            <el-table-column label="操作" width="200" fixed="right">
              <template #default="{ row }">
                <el-button link type="primary" :loading="reuploadingId === row.id" @click="pickReupload(row.id)">重传</el-button>
                <el-button link type="primary" @click="viewDoc(row)">查看</el-button>
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
    <input
      ref="reuploadInput"
      class="reupload-input"
      type="file"
      accept=".txt,.md,.markdown,.csv,.json,.xml,.html,.htm,.pdf,.docx,.png,.jpg,.jpeg,.webp,.bmp,.tif,.tiff,.gif"
      @change="onReuploadFile"
    />
    <el-drawer v-model="textOpen" :title="textTitle" size="46%" destroy-on-close>
      <p v-if="textSource && textSource !== textTitle" class="muted drawer-source">{{ textSource }}</p>
      <p v-if="textLoading" class="muted">加载中…</p>
      <p v-else-if="!textContent" class="muted">暂无正文</p>
      <div v-else class="md-body doc-body" v-html="textHtml" />
    </el-drawer>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { useRoute } from "vue-router";
import { ElMessage, ElMessageBox } from "element-plus";
import type { UploadFile, UploadRawFile, UploadUserFile } from "element-plus";
import { Collection, Document, Refresh, Upload } from "@element-plus/icons-vue";
import { apiURL, requestJSON, requestForm, downloadFile, formatAPIError } from "@/api/client";
import { renderMarkdown } from "@/lib/markdown";
import PageHero, { type HeroItem } from "@/components/PageHero.vue";
import type { Corpus, Document as Doc } from "@/api/types";

const hero: HeroItem[] = [
  { icon: Collection, title: "语料库", desc: "按业务划分，名称唯一，绑定 Embedding 模型", tone: "blue" },
  { icon: Upload, title: "分别索引", desc: "多文件不合并，每篇可单独删除或重传", tone: "green" },
  { icon: Document, title: "自动分块", desc: "入库后切分并写入 pgvector 向量列", tone: "purple" },
  { icon: Refresh, title: "重建索引", desc: "更换 Embedding 模型后需要重新索引", tone: "orange" },
];

const route = useRoute();
const corpora = ref<Corpus[]>([]);
const current = ref<Corpus | null>(null);
const docs = ref<Doc[]>([]);
const docTitle = ref("");
const docContent = ref("");
const fileList = ref<UploadUserFile[]>([]);
const uploading = ref(false);
const reuploadingId = ref("");
const reuploadDocId = ref("");
const reuploadInput = ref<HTMLInputElement | null>(null);
const textOpen = ref(false);
const textLoading = ref(false);
const textTitle = ref("");
const textSource = ref("");
const textContent = ref("");
const textHtml = computed(() => renderMarkdown(textContent.value));

const pendingFiles = computed(() =>
  fileList.value.map((f) => f.raw).filter((f): f is UploadRawFile => f != null),
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
  const corpusID = current.value.id;
  let ok = 0;
  const failed: string[] = [];
  const failedUIDs = new Set<number>();
  try {
    for (const f of files) {
      const fd = new FormData();
      fd.append("file", f, f.name);
      try {
        await requestForm(`/api/v1/corpora/${corpusID}/documents`, fd);
        ok++;
      } catch (e) {
        failed.push(`${f.name}: ${formatAPIError(e)}`);
        failedUIDs.add(f.uid);
      }
    }
    fileList.value = fileList.value.filter((item) => item.raw != null && failedUIDs.has(item.raw.uid));
    await loadDocs(corpusID);
    if (failed.length) {
      ElMessage.warning(`已单独索引 ${ok} 个，失败 ${failed.length}。${failed.join("；")}`);
    } else {
      ElMessage.success(ok > 1 ? `已分别索引 ${ok} 个文件` : "已上传");
    }
  } finally {
    uploading.value = false;
  }
}

function pickReupload(docId: string) {
  reuploadDocId.value = docId;
  const input = reuploadInput.value;
  if (!input) return;
  input.value = "";
  input.click();
}

async function onReuploadFile(ev: Event) {
  const input = ev.target as HTMLInputElement;
  const file = input.files?.[0];
  const docId = reuploadDocId.value;
  input.value = "";
  if (!current.value || !file || !docId) return;
  reuploadingId.value = docId;
  try {
    const fd = new FormData();
    fd.append("file", file, file.name);
    await requestForm(`/api/v1/corpora/${current.value.id}/documents/${docId}/reupload`, fd);
    ElMessage.success("已重新索引该文件");
    await loadDocs(current.value.id);
  } catch (e) {
    ElMessage.error(formatAPIError(e));
  } finally {
    reuploadingId.value = "";
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

const textExts = new Set(["txt", "md", "markdown", "csv", "json", "xml", "html", "htm"]);
const previewExts = new Set(["pdf", "png", "jpg", "jpeg", "webp", "gif", "bmp", "tif", "tiff"]);

function fileExt(name: string): string {
  const base = name.split(/[/\\]/).pop() || "";
  const dot = base.lastIndexOf(".");
  return dot >= 0 ? base.slice(dot + 1).toLowerCase() : "";
}

function isTextDocument(row: Doc): boolean {
  const ext = fileExt(row.source || row.title || "");
  if (textExts.has(ext)) return true;
  if (row.kind === "file") return false;
  return row.kind === "text" || ext === "";
}

async function viewDoc(row: Doc) {
  if (!current.value) return;
  if (isTextDocument(row)) {
    const corpusID = current.value.id;
    textOpen.value = true;
    textLoading.value = true;
    textTitle.value = row.title || "未命名";
    textSource.value = row.source || "";
    textContent.value = "";
    try {
      const data = await requestJSON<{ document: Doc; content: string }>(
        `/api/v1/corpora/${corpusID}/documents/${row.id}`,
      );
      textTitle.value = data.document?.title || textTitle.value;
      textSource.value = data.document?.source || textSource.value;
      textContent.value = data.content || "";
    } catch (e) {
      textOpen.value = false;
      ElMessage.error(formatAPIError(e));
    } finally {
      textLoading.value = false;
    }
    return;
  }
  const path = `/api/v1/corpora/${current.value.id}/documents/${row.id}/file`;
  const name = row.source || row.title;
  if (previewExts.has(fileExt(name))) {
    window.open(apiURL(path), "_blank", "noopener");
    return;
  }
  downloadFile(path, name).catch((e) => ElMessage.error(formatAPIError(e)));
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
    const id = typeof route.query.id === "string" ? route.query.id : "";
    if (!id) return;
    const row = corpora.value.find((c) => c.id === id);
    if (row) await onSelect(row);
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

.reupload-input {
  display: none;
}

.drawer-source {
  margin-top: 0;
}

.doc-body {
  line-height: 1.7;
}
</style>
