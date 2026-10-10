<template>
  <div class="page">
    <PageHero :items="hero" />

    <div class="panel" style="max-width: 720px">
      <p class="panel-title">连接参数</p>
      <el-form label-width="120px" @submit.prevent>
        <el-form-item label="API Base">
          <el-input v-model="form.apiBase" placeholder="留空则走 Vite 代理（/api → VITE_PROXY_TARGET）" />
          <div class="muted">直连后端时填写如 http://127.0.0.1:18090；默认 CORS 允许所有来源</div>
        </el-form-item>
        <el-form-item label="X-User-Id">
          <el-input v-model="form.userId" placeholder="会话归属，写入新建会话的 uid" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="save">保存到本机</el-button>
          <el-button :loading="pinging" @click="ping">测试连接</el-button>
        </el-form-item>
      </el-form>
      <el-alert v-if="msg" :title="msg" :type="ok ? 'success' : 'error'" show-icon :closable="false" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { reactive, ref } from "vue";
import { ElMessage } from "element-plus";
import { Connection, Link, User } from "@element-plus/icons-vue";
import PageHero, { type HeroItem } from "@/components/PageHero.vue";
import { useSettingsStore } from "@/stores/settings";
import { useModelsStore } from "@/stores/models";
import { formatAPIError } from "@/api/client";

const settings = useSettingsStore();
const models = useModelsStore();
const form = reactive({
  apiBase: settings.apiBase,
  userId: settings.userId,
});
const pinging = ref(false);
const msg = ref("");
const ok = ref(false);

const hero: HeroItem[] = [
  { icon: Link, title: "后端地址", desc: "留空走开发代理，填写则浏览器直连", tone: "blue" },
  { icon: User, title: "用户标识", desc: "X-User-Id 决定新建会话的归属", tone: "purple" },
  { icon: Connection, title: "本机保存", desc: "参数存在浏览器 localStorage，不上传", tone: "orange" },
];

function save() {
  settings.save(form);
  void models.refresh();
  ElMessage.success("已保存");
}

async function ping() {
  settings.save(form);
  pinging.value = true;
  msg.value = "";
  try {
    await models.refresh();
    if (!models.health) throw new Error(models.error || "无法访问 /health");
    ok.value = true;
    msg.value = `健康检查：status=${models.health.status} db=${models.health.db}；厂商 ${models.providers.length} 个`;
  } catch (e) {
    ok.value = false;
    msg.value = formatAPIError(e);
  } finally {
    pinging.value = false;
  }
}
</script>
