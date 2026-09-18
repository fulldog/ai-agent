<template>
  <div class="hero-cards">
    <div v-for="item in items" :key="item.title" class="hero-card">
      <div class="hero-icon" :class="item.tone || 'blue'">
        <el-icon><component :is="item.icon" /></el-icon>
      </div>
      <div>
        <div class="hero-title">{{ item.title }}</div>
        <div class="hero-desc">{{ item.desc }}</div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { Component } from "vue";

export interface HeroItem {
  icon: Component;
  title: string;
  desc: string;
  tone?: "blue" | "green" | "purple" | "orange";
}

defineProps<{ items: HeroItem[] }>();
</script>
