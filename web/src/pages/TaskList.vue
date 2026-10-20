<template>
  <div class="task-list">
    <PageHeading title="任务"
      ><GlassButton label="创建任务" tone="accent" to="/tasks/create"
    /></PageHeading>
    <div class="filters">
      <div class="filter-fields">
        <BaseSelect
          v-model="filterType"
          label="任务类型"
          :options="taskTypeFilters"
          :disabled="loading"
          @change="fetchTasks(1)"
        />
        <BaseSelect
          v-model="filterPhase"
          label="任务状态"
          :options="taskPhases"
          :disabled="loading"
          @change="fetchTasks(1)"
        />
      </div>
      <div class="filter-actions">
        <span class="filter-caption">{{ total }} 个任务</span>
        <GlassButton
          label="刷新"
          :disabled="loading"
          @click="fetchTasks(page)"
        />
      </div>
    </div>
    <div v-if="loading" class="loading">加载中…</div>
    <NoCluster v-else-if="clusterError" :message="clusterError" />
    <template v-else>
      <p v-if="error" class="error-msg" role="alert">{{ error }}</p>
      <div v-if="tasks.length" class="table-panel content-panel">
        <div class="table-wrap">
          <table>
            <thead>
              <tr>
                <th>名称</th>
                <th>类型</th>
                <th>状态</th>
                <th>镜像</th>
                <th>上次执行</th>
                <th>成功 / 失败</th>
                <th>操作</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="task in tasks" :key="task.metadata.name">
                <td>
                  <router-link
                    :to="{
                      name: 'TaskDetail',
                      params: { name: task.metadata.name },
                    }"
                    class="resource-name"
                    :title="task.metadata.name"
                    >{{ task.metadata.name }}</router-link
                  >
                </td>
                <td>
                  <span class="tag type">{{ task.spec?.type }}</span>
                </td>
                <td>
                  <span class="tag" :class="phaseClass(task.status?.phase)">{{
                    task.status?.phase || "Pending"
                  }}</span>
                </td>
                <td class="img-cell" :title="task.spec?.image">
                  {{ task.spec?.image }}
                </td>
                <td class="time-cell">
                  {{ formatTime(task.status?.lastStartTime) }}
                </td>
                <td class="count-cell">
                  <span class="green">{{ task.status?.succeeded || 0 }}</span> /
                  <span class="red">{{ task.status?.failed || 0 }}</span>
                </td>
                <td class="actions">
                  <router-link
                    class="compact-action"
                    :to="{
                      name: 'TaskDetail',
                      params: { name: task.metadata.name },
                    }"
                    :aria-label="'查看 ' + task.metadata.name + ' 详情'"
                    >查看</router-link
                  >
                  <button
                    :disabled="acting === task.metadata.name"
                    @click="doTrigger(task)"
                    title="触发"
                    aria-label="触发任务"
                  >
                    <AppIcon name="play" /></button
                  ><button
                    v-if="task.spec?.suspend"
                    :disabled="acting === task.metadata.name"
                    @click="doResume(task)"
                    title="恢复"
                    aria-label="恢复任务"
                  >
                    <AppIcon name="refresh" /></button
                  ><button
                    v-else
                    :disabled="acting === task.metadata.name"
                    @click="doSuspend(task)"
                    title="暂停"
                    aria-label="暂停任务"
                  >
                    <AppIcon name="pause" /></button
                  ><router-link
                    :to="{
                      name: 'TaskLogs',
                      params: { name: task.metadata.name },
                    }"
                    class="btn-link"
                    title="日志"
                    aria-label="查看日志"
                    ><AppIcon name="logs" /></router-link
                  ><button
                    :disabled="acting === task.metadata.name"
                    @click="doDelete(task)"
                    class="danger"
                    title="删除"
                    aria-label="删除任务"
                  >
                    <AppIcon name="trash" />
                  </button>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
      <div v-else-if="!error" class="empty content-panel">
        暂无任务。<router-link to="/tasks/create">创建任务</router-link>
      </div>
      <div v-if="total > PAGE_SIZE" class="pager">
        <button :disabled="page <= 1" @click="fetchTasks(page - 1)">
          上一页</button
        ><span>{{ page }} / {{ totalPages }} · 共 {{ total }} 条</span
        ><button :disabled="page >= totalPages" @click="fetchTasks(page + 1)">
          下一页
        </button>
      </div>
    </template>
  </div>
</template>
<script setup>
import { ref, computed, onMounted } from "vue";
import { useRoute } from "vue-router";
import PageHeading from "@/components/PageHeading.vue";
import GlassButton from "@/components/GlassButton.vue";
import AppIcon from "@/components/AppIcon.vue";
import BaseSelect from "@/components/BaseSelect.vue";
import { taskTypeFilters, taskPhases } from "@/components/select-options.js";
import { api } from "@/api";
import { useClusterStatus } from "@/composables/cluster";
import NoCluster from "@/components/NoCluster.vue";

const { clusterError, handleApiError } = useClusterStatus();

const PAGE_SIZE = 10;

const loading = ref(true);
const error = ref("");
const acting = ref("");
const tasks = ref([]);
const page = ref(1);
const total = ref(0);
const filterType = ref("");
const route = useRoute();
const filterPhase = ref(
  ["Pending", "Running", "Succeeded", "Failed", "Suspended"].includes(
    route.query.phase,
  )
    ? route.query.phase
    : "",
);

const totalPages = computed(() => Math.ceil(total.value / PAGE_SIZE));

onMounted(() => fetchTasks(1));

async function fetchTasks(p) {
  loading.value = true;
  error.value = "";
  try {
    const params = { page: p, pageSize: PAGE_SIZE };
    if (filterType.value) params.type = filterType.value;
    if (filterPhase.value) params.phase = filterPhase.value;
    const data = await api.listTasks(params);
    tasks.value = data.items || [];
    total.value = data.total || 0;
    page.value = data.page || p;
  } catch (e) {
    if (handleApiError(e) !== null) {
      error.value = "加载任务列表失败: " + handleApiError(e);
    }
  } finally {
    loading.value = false;
  }
}

async function doAction(t, action) {
  acting.value = t.metadata.name;
  try {
    await action(t);
    await fetchTasks(page.value);
  } catch (e) {
    error.value = "操作失败: " + e.message;
  } finally {
    acting.value = "";
  }
}

function doTrigger(t) {
  if (!confirm("确认触发 " + t.metadata.name + "？")) return;
  doAction(t, (t) => api.triggerTask(t.metadata.name));
}
function doSuspend(t) {
  doAction(t, (t) => api.suspendTask(t.metadata.name));
}
function doResume(t) {
  doAction(t, (t) => api.resumeTask(t.metadata.name));
}
function doDelete(t) {
  if (!confirm("确认删除 " + t.metadata.name + "？")) return;
  doAction(t, (t) => api.deleteTask(t.metadata.name));
}

function phaseClass(phase) {
  const map = {
    Running: "running",
    Succeeded: "succeeded",
    Failed: "failed",
    Suspended: "suspended",
    Pending: "pending",
  };
  return map[phase] || "pending";
}

function formatTime(t) {
  if (!t) return "-";
  return new Date(t).toLocaleString("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  });
}
</script>
