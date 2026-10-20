<template>
  <div class="detail">
    <PageHeading
      :title="String(route.params.name)"
      back-to="/tasks"
      back-label="任务"
    >
      <template v-if="task && !loading">
        <GlassButton
          label="触发"
          tone="accent"
          @click="doTrigger"
          :disabled="acting"
        />
        <GlassButton
          label="查看日志"
          :to="'/tasks/' + task.metadata.name + '/logs'"
        />
        <details
          class="action-menu"
          @focusout="closeActionMenu"
          @keydown.esc="$event.currentTarget.open = false"
        >
          <summary>更多</summary>
          <div class="action-menu-items">
            <button
              v-if="task.spec?.suspend"
              @click="doResume"
              :disabled="acting"
            >
              恢复
            </button>
            <button v-else @click="doSuspend" :disabled="acting">暂停</button>
            <button @click="doDelete" class="danger" :disabled="acting">
              删除
            </button>
          </div>
        </details>
      </template>
    </PageHeading>
    <div v-if="loading" class="loading">加载中...</div>
    <NoCluster v-else-if="clusterError" :message="clusterError" />
    <div v-else-if="error" class="error-msg">{{ error }}</div>

    <template v-else-if="task">
      <div class="row">
        <!-- 基本信息 -->
        <div class="card glass-panel">
          <h2>基本信息</h2>
          <dl>
            <dt>类型</dt>
            <dd>
              <span class="tag type">{{ task.spec?.type }}</span>
            </dd>
            <dt>状态</dt>
            <dd>
              <span class="tag" :class="phaseClass(task.status?.phase)">{{
                task.status?.phase || "Pending"
              }}</span>
            </dd>
            <dt>镜像</dt>
            <dd>
              <code>{{ task.spec?.image }}</code>
            </dd>
            <dt v-if="task.spec?.command">命令</dt>
            <dd v-if="task.spec?.command">
              <code>{{ task.spec.command.join(" ") }}</code>
            </dd>
            <dt v-if="task.spec?.schedule">Cron</dt>
            <dd v-if="task.spec?.schedule">
              <code>{{ task.spec.schedule }}</code>
            </dd>
            <dt v-if="task.spec?.delay">延迟</dt>
            <dd v-if="task.spec?.delay">
              <code>{{ task.spec.delay }}</code>
            </dd>
            <dt>成功</dt>
            <dd class="green">{{ task.status?.succeeded || 0 }}</dd>
            <dt>失败</dt>
            <dd class="red">{{ task.status?.failed || 0 }}</dd>
            <dt>创建时间</dt>
            <dd>{{ formatTime(task.metadata.creationTimestamp) }}</dd>
            <dt v-if="task.status?.lastStartTime">上次执行</dt>
            <dd v-if="task.status?.lastStartTime">
              {{ formatTime(task.status.lastStartTime) }}
            </dd>
            <dt v-if="task.status?.lastCompletionTime">上次完成</dt>
            <dd v-if="task.status?.lastCompletionTime">
              {{ formatTime(task.status.lastCompletionTime) }}
            </dd>
          </dl>
        </div>

        <!-- YAML 预览 -->
        <div class="card glass-panel">
          <h2>YAML</h2>
          <pre class="yaml">{{ yamlPreview }}</pre>
        </div>
      </div>

      <!-- 执行历史 -->
      <div
        class="card glass-panel"
        v-if="task.status?.executionHistory?.length"
      >
        <h2>执行历史</h2>
        <div class="table-wrap">
          <table>
            <thead>
              <tr>
                <th>Job</th>
                <th>开始时间</th>
                <th>结束时间</th>
                <th>结果</th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="rec in task.status.executionHistory.slice().reverse()"
                :key="rec.jobName"
              >
                <td>
                  <code>{{ rec.jobName }}</code>
                </td>
                <td>{{ formatTime(rec.startTime) }}</td>
                <td>{{ formatTime(rec.stopTime) }}</td>
                <td>
                  <span class="tag" :class="phaseClass(rec.phase)">{{
                    rec.phase
                  }}</span>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </template>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from "vue";
import { useRoute, useRouter } from "vue-router";
import { api } from "@/api";
import { useClusterStatus } from "@/composables/cluster";
import NoCluster from "@/components/NoCluster.vue";
import PageHeading from "@/components/PageHeading.vue";
import GlassButton from "@/components/GlassButton.vue";

const route = useRoute();
const router = useRouter();
const { clusterError, handleApiError } = useClusterStatus();

const loading = ref(true);
const error = ref("");
const acting = ref(false);
const task = ref(null);

function closeActionMenu(event) {
  if (!event.currentTarget.contains(event.relatedTarget))
    event.currentTarget.open = false;
}

const yamlPreview = computed(() => {
  if (!task.value) return "";
  return `apiVersion: kubetask.kubetask.io/v1
kind: Task
metadata:
  name: ${task.value.metadata.name}
  creationTimestamp: ${formatTime(task.value.metadata.creationTimestamp)}
spec:
  type: ${task.value.spec?.type}
  image: ${task.value.spec?.image}
  ${task.value.spec?.command ? "command:\n" + task.value.spec.command.map((c) => "    - " + c).join("\n") : "command: []"}
  backoffLimit: ${task.value.spec?.backoffLimit || 3}
status:
  phase: ${task.value.status?.phase || "Pending"}
  succeeded: ${task.value.status?.succeeded || 0}
  failed: ${task.value.status?.failed || 0}`;
});

onMounted(() => fetchTask());

async function fetchTask() {
  loading.value = true;
  try {
    task.value = await api.getTask(route.params.name);
  } catch (e) {
    if (handleApiError(e) !== null) error.value = "加载失败: " + e.message;
  } finally {
    loading.value = false;
  }
}

async function doAction(action) {
  acting.value = true;
  try {
    await action();
    await fetchTask();
  } catch (e) {
    if (handleApiError(e) !== null) error.value = "操作失败: " + e.message;
  } finally {
    acting.value = false;
  }
}
function doTrigger() {
  doAction(() => api.triggerTask(route.params.name));
}
function doSuspend() {
  doAction(() => api.suspendTask(route.params.name));
}
function doResume() {
  doAction(() => api.resumeTask(route.params.name));
}
function doDelete() {
  if (!confirm("确认删除？")) return;
  doAction(async () => {
    await api.deleteTask(route.params.name);
    router.push("/tasks");
  });
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
    second: "2-digit",
  });
}
</script>
