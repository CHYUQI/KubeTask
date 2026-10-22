<template>
  <div class="workflow-list">
    <PageHeading title="工作流"
      ><GlassButton label="创建工作流" tone="accent" to="/workflows/create"
    /></PageHeading>
    <div class="filters">
      <div class="filter-fields">
        <BaseSelect
          v-model="filterPhase"
          label="工作流状态"
          :options="workflowPhases"
          :disabled="loading"
          @change="fetchWorkflows(1)"
        />
      </div>
      <div class="filter-actions">
        <span class="filter-caption">{{ total }} 个工作流</span>
        <GlassButton
          label="刷新"
          :disabled="loading"
          @click="fetchWorkflows(page)"
        />
      </div>
    </div>
    <div v-if="loading" class="loading">加载中…</div>
    <NoCluster v-else-if="clusterError" :message="clusterError" />
    <GlassContainer as="p" v-else-if="error" class="error-msg" role="alert">{{ error }}</GlassContainer>
    <template v-else>
      <GlassContainer v-if="workflows.length" class="table-panel">
        <div class="table-wrap">
          <table>
            <thead>
              <tr>
                <th>名称</th>
                <th>状态</th>
                <th>节点</th>
                <th>并行上限</th>
                <th>创建时间</th>
                <th>开始时间</th>
                <th>完成时间</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="workflow in workflows" :key="workflow.metadata.name">
                <td>
                  <router-link
                    :to="{
                      name: 'WorkflowDetail',
                      params: { name: workflow.metadata.name },
                    }"
                    >{{ workflow.metadata.name }}</router-link
                  >
                </td>
                <td>
                  <span
                    class="tag"
                    :class="phaseClass(workflow.status?.phase)"
                    >{{ workflow.status?.phase || "未初始化" }}</span
                  ><span
                    v-if="workflow.metadata.deletionTimestamp"
                    class="deleting"
                    >删除中</span
                  >
                </td>
                <td>{{ workflow.spec?.tasks?.length || 0 }}</td>
                <td>{{ workflow.spec?.maxParallel || "不限" }}</td>
                <td class="time-cell">
                  {{ formatTime(workflow.metadata.creationTimestamp) }}
                </td>
                <td class="time-cell">
                  {{ formatTime(workflow.status?.startTime) }}
                </td>
                <td class="time-cell">
                  {{ formatTime(workflow.status?.completionTime) }}
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </GlassContainer>
      <GlassContainer v-else class="empty">
        {{ filterPhase ? "没有符合当前状态的工作流。" : "暂无工作流。"
        }}<router-link v-if="!filterPhase" to="/workflows/create"
          >创建工作流</router-link
        >
      </GlassContainer>
      <div v-if="total > PAGE_SIZE" class="pager">
        <button :disabled="page <= 1" @click="fetchWorkflows(page - 1)">
          上一页</button
        ><span>{{ page }} / {{ totalPages }} · 共 {{ total }} 条</span
        ><button
          :disabled="page >= totalPages"
          @click="fetchWorkflows(page + 1)"
        >
          下一页
        </button>
      </div>
    </template>
  </div>
</template>
<script setup>
import GlassContainer from "@/components/GlassContainer.vue";
import { ref, computed, onMounted } from "vue";
import PageHeading from "@/components/PageHeading.vue";
import GlassButton from "@/components/GlassButton.vue";
import BaseSelect from "@/components/BaseSelect.vue";
import { workflowPhases } from "@/components/select-options.js";
import { api } from "@/api";
import { useClusterStatus } from "@/composables/cluster";
import NoCluster from "@/components/NoCluster.vue";

const PAGE_SIZE = 10;
const { clusterError, handleApiError } = useClusterStatus();
const loading = ref(true);
const error = ref("");
const workflows = ref([]);
const filterPhase = ref("");
const page = ref(1);
const total = ref(0);
const totalPages = computed(() =>
  Math.max(1, Math.ceil(total.value / PAGE_SIZE)),
);

onMounted(() => fetchWorkflows(1));

async function fetchWorkflows(p) {
  loading.value = true;
  error.value = "";
  clusterError.value = "";
  try {
    const params = { page: p, pageSize: PAGE_SIZE };
    if (filterPhase.value) params.phase = filterPhase.value;
    const data = await api.listWorkflows(params);
    const lastPage = Math.max(1, Math.ceil(data.total / PAGE_SIZE));
    // 工作流被删除后，刷新时返回仍有数据的最后一页。
    if (p > lastPage) {
      await fetchWorkflows(lastPage);
      return;
    }
    workflows.value = data.items || [];
    total.value = data.total || 0;
    page.value = data.page || p;
  } catch (e) {
    const message = handleApiError(e);
    if (message !== null) error.value = "加载工作流列表失败: " + message;
  } finally {
    loading.value = false;
  }
}

function phaseClass(phase) {
  return (
    {
      Pending: "pending",
      Running: "running",
      Succeeded: "succeeded",
      Failed: "failed",
    }[phase] || "pending"
  );
}

function formatTime(value) {
  if (!value) return "-";
  return new Date(value).toLocaleString("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  });
}
</script>
