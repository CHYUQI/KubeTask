<template>
  <div class="logs">
    <PageHeading
      title="执行日志"
      :description="taskName"
      :back-to="'/tasks/' + taskName"
      back-label="任务详情"
    />
    <div class="log-toolbar">
      <div class="controls">
        <label
          >行数 <input v-model.number="tail" type="number" min="1" max="10000"
        /></label>
        <label class="follow-label">
          <input v-model="follow" type="checkbox" />
          实时跟踪
        </label>
        <GlassButton
          :label="streaming ? '停止' : '开始'"
          :tone="streaming ? 'neutral' : 'accent'"
          @click="toggleStream"
        />
      </div>
    </div>

    <!-- 搜索栏 -->
    <div v-if="lines.length > 0" class="search-bar">
      <input
        v-model="searchQuery"
        aria-label="搜索日志"
        placeholder="搜索日志…"
      />
      <span v-if="searchQuery" class="match-info">
        {{ filteredLines.length }}/{{ lines.length }} 条匹配
      </span>
      <GlassButton v-if="searchQuery" label="清除" @click="searchQuery = ''" />
    </div>

    <NoCluster v-if="clusterError" :message="clusterError" />
    <GlassContainer v-else-if="error" class="error-msg">{{ error }}</GlassContainer>

    <GlassContainer v-if="!clusterError" class="log-surface">
      <div
        ref="logContainer"
        class="log-container"
        @scroll="onScroll"
      >
        <div v-if="!streaming && lines.length === 0" class="hint">
          点击「开始」加载日志
        </div>
        <div
          v-for="item in filteredLines"
          :key="item.index"
          class="log-line"
          :class="lineClass(item.text)"
        >
          <span class="line-no">{{ item.index + 1 }}</span>
          <span class="line-text" v-html="highlightText(item.text)"></span>
        </div>
        <div
          v-if="searchQuery && filteredLines.length === 0 && lines.length > 0"
          class="hint"
        >
          无匹配结果
        </div>
      </div>
    </GlassContainer>
  </div>
</template>

<script setup>
import GlassContainer from "@/components/GlassContainer.vue";
import { ref, computed, onUnmounted } from "vue";
import PageHeading from "@/components/PageHeading.vue";
import GlassButton from "@/components/GlassButton.vue";
import { useRoute } from "vue-router";
import { useClusterStatus } from "@/composables/cluster";
import NoCluster from "@/components/NoCluster.vue";
import { streamTaskLogs } from "@/api/logs.js";

const route = useRoute();
const taskName = route.params.name;
const { clusterError, handleApiError } = useClusterStatus();

const error = ref("");
const streaming = ref(false);
const tail = ref(200);
const follow = ref(false);
const lines = ref([]);
const searchQuery = ref("");
const logContainer = ref(null);
const autoScroll = ref(true);

let streamController = null;
let scrollFrame = 0;

const filteredLines = computed(() => {
  if (!searchQuery.value)
    return lines.value.map((text, index) => ({ index, text }));
  const q = searchQuery.value.toLowerCase();
  return lines.value
    .map((text, index) => ({ index, text }))
    .filter((item) => item.text.toLowerCase().includes(q));
});

function escapeHtml(text) {
  return text
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;");
}

function highlightText(text) {
  const escaped = escapeHtml(text);
  if (!searchQuery.value) return escaped;
  const q = searchQuery.value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  return escaped.replace(new RegExp(`(${q})`, "gi"), "<mark>$1</mark>");
}

function toggleStream() {
  if (streaming.value) {
    stopStream();
  } else {
    startStream();
  }
}

async function startStream() {
  stopStream();
  error.value = "";
  clusterError.value = "";
  searchQuery.value = "";
  lines.value = [];
  streaming.value = true;

  const controller = new AbortController();
  streamController = controller;
  try {
    await streamTaskLogs(taskName, {
      tail: tail.value,
      follow: follow.value,
      signal: controller.signal,
      onMessage(text) {
        if (streamController !== controller) return;
        lines.value.push(text);
        if (autoScroll.value) scrollBottom();
      },
    });
  } catch (e) {
    if (streamController === controller && e.name !== "AbortError") {
      const message = handleApiError(e);
      if (message !== null) error.value = message;
    }
  } finally {
    if (streamController === controller) {
      streaming.value = false;
      streamController = null;
    }
  }
}

function stopStream() {
  streaming.value = false;
  streamController?.abort();
  streamController = null;
}

function scrollBottom() {
  if (scrollFrame) return;
  scrollFrame = requestAnimationFrame(() => {
    scrollFrame = 0;
    const el = logContainer.value;
    if (el && autoScroll.value) el.scrollTop = el.scrollHeight;
  });
}

function onScroll() {
  if (!logContainer.value) return;
  const el = logContainer.value;
  autoScroll.value = el.scrollHeight - el.scrollTop - el.clientHeight < 50;
}

function lineClass(line) {
  const lower = line.toLowerCase();
  if (
    lower.includes("error") ||
    lower.includes("fail") ||
    lower.includes("fatal")
  )
    return "level-error";
  if (lower.includes("warn")) return "level-warn";
  return "";
}

onUnmounted(() => {
  stopStream();
  cancelAnimationFrame(scrollFrame);
});
</script>
