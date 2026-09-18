<template>
  <div class="page">
    <PageHero :items="hero" />

    <div class="toolbar">
      <el-form inline @submit.prevent>
        <el-form-item>
          <el-button type="primary" :loading="models.loading" @click="models.refresh()">刷新</el-button>
        </el-form-item>
      </el-form>
      <div class="toolbar-summary">厂商 {{ models.providers.length }} 个，已配置 Key {{ configuredCount }} 个</div>
    </div>

    <el-row :gutter="12">
      <el-col :span="8">
        <div class="panel">
          <p class="panel-title">服务健康</p>
          <el-descriptions v-if="models.health" :column="1" border size="small">
            <el-descriptions-item label="status">
              <span><i class="status-dot" :class="models.health.status === 'ok' ? 'ok' : 'err'"></i>{{ models.health.status }}</span>
            </el-descriptions-item>
            <el-descriptions-item label="db">{{ models.health.db }}</el-descriptions-item>
            <el-descriptions-item v-if="models.health.mode" label="mode">{{ models.health.mode }}</el-descriptions-item>
          </el-descriptions>
          <p v-else-if="!models.loading" class="muted">尚未拉取 /health，请先填写 API Key 并刷新</p>
          <el-alert
            v-if="models.error"
            :title="models.error"
            type="error"
            show-icon
            :closable="false"
            style="margin-top: 12px"
          />
        </div>
      </el-col>
      <el-col :span="16">
        <div class="panel">
          <p class="panel-title">已配置 LLM 厂商</p>
          <el-table :data="models.providers" stripe empty-text="无数据">
            <el-table-column prop="name" label="厂商" width="140" />
            <el-table-column prop="default_model" label="默认模型" min-width="160" />
            <el-table-column prop="base_url" label="Base URL" min-width="220" show-overflow-tooltip />
            <el-table-column label="Key" width="100">
              <template #default="{ row }">
                <el-tag size="small" :type="row.configured ? 'success' : 'info'" effect="light">
                  {{ row.configured ? "已配置" : "未配置" }}
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column label="启用" width="100">
              <template #default="{ row }">
                <span><i class="status-dot" :class="row.enabled ? 'ok' : ''"></i>{{ row.enabled ? "启用" : "停用" }}</span>
              </template>
            </el-table-column>
          </el-table>
        </div>
      </el-col>
    </el-row>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted } from "vue";
import { Cpu, Key, Monitor, Platform } from "@element-plus/icons-vue";
import PageHero, { type HeroItem } from "@/components/PageHero.vue";
import { useModelsStore } from "@/stores/models";

const models = useModelsStore();

const hero: HeroItem[] = [
  { icon: Monitor, title: "服务健康", desc: "读取 /health，展示数据库连通性与部署模式", tone: "blue" },
  { icon: Cpu, title: "模型厂商", desc: "列出 YAML 中配置的 OpenAI 兼容上游", tone: "green" },
  { icon: Key, title: "鉴权方式", desc: "请求头 X-API-Key，会话隔离用 X-User-Id", tone: "purple" },
  { icon: Platform, title: "最小化部署", desc: "未连库时仅文件分析、意图解析可用", tone: "orange" },
];

const configuredCount = computed(() => models.providers.filter((p) => p.configured).length);

onMounted(() => {
  models.refresh().catch(() => undefined);
});
</script>
