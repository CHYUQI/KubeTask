<template>
  <div class="workflow-create page-compact">
    <PageHeading title="创建工作流" back-to="/workflows" back-label="工作流" />
    <form class="form content-panel" @submit.prevent="submit">
      <div class="form-section">
        <h2>基本信息</h2>
        <div class="form-row">
          <div class="form-group">
            <label for="workflow-name">名称 <span class="opt">可选</span></label
            ><input
              id="workflow-name"
              v-model="name"
              placeholder="留空自动生成"
              :disabled="submitting"
            />
          </div>
          <div class="form-group">
            <label for="workflow-parallel">并行上限</label
            ><input
              id="workflow-parallel"
              v-model.number="maxParallel"
              type="number"
              min="0"
              step="1"
              required
              :disabled="submitting"
            />
            <p class="hint">0 表示不限制。</p>
          </div>
        </div>
      </div>
      <div class="form-section">
        <h2>节点</h2>
        <div class="form-group">
          <label for="workflow-tasks">节点定义 · JSON</label
          ><textarea
            id="workflow-tasks"
            v-model="tasksJSON"
            class="code-input"
            rows="18"
            spellcheck="false"
            required
            :disabled="submitting"
          />
          <p class="hint">
            使用 dependsOn 声明依赖。每个节点设置 taskSpec 或 template，内联
            taskSpec 的类型为 OneTime。
          </p>
        </div>
        <details class="template-options">
          <summary>Task 模板（可选）</summary>
          <div class="form-group">
            <label for="workflow-templates">taskTemplates · JSON</label
            ><textarea
              id="workflow-templates"
              v-model="templatesJSON"
              class="code-input"
              rows="6"
              spellcheck="false"
              :disabled="submitting"
            />
          </div>
        </details>
      </div>
      <NoCluster v-if="clusterError" :message="clusterError" />
      <p v-if="error" class="error-msg" role="alert">{{ error }}</p>
      <div class="form-actions">
        <span class="hint">创建后开始执行。</span
        ><GlassButton
          type="submit"
          tone="accent"
          :label="submitting ? '创建中…' : '创建工作流'"
          :disabled="submitting"
        />
      </div>
    </form>
  </div>
</template>

<script setup>
import { onUnmounted, ref } from "vue";
import { useRouter } from "vue-router";
import { api } from "@/api";
import { useClusterStatus } from "@/composables/cluster";
import PageHeading from "@/components/PageHeading.vue";
import GlassButton from "@/components/GlassButton.vue";
import NoCluster from "@/components/NoCluster.vue";

const router = useRouter();
const { clusterError, handleApiError } = useClusterStatus();
const name = ref(""),
  maxParallel = ref(0),
  error = ref(""),
  submitting = ref(false);
const tasksJSON = ref(
  JSON.stringify(
    [
      {
        name: "prepare",
        taskSpec: {
          type: "OneTime",
          image: "busybox:1.37",
          command: ["sh", "-c", "echo prepare"],
        },
      },
      {
        name: "run",
        dependsOn: ["prepare"],
        taskSpec: {
          type: "OneTime",
          image: "busybox:1.37",
          command: ["sh", "-c", "echo run"],
        },
      },
    ],
    null,
    2,
  ),
);
const templatesJSON = ref("{}");
let disposed = false;
onUnmounted(() => {
  disposed = true;
});

async function submit() {
  if (submitting.value) return;
  error.value = "";
  clusterError.value = "";
  let tasks, templates;
  try {
    tasks = JSON.parse(tasksJSON.value);
  } catch {
    error.value = "节点定义不是有效的 JSON。";
    return;
  }
  if (!Array.isArray(tasks) || !tasks.length) {
    error.value = "节点定义必须是包含至少一个节点的 JSON 数组。";
    return;
  }
  try {
    templates = JSON.parse(templatesJSON.value || "{}");
  } catch {
    error.value = "Task 模板不是有效的 JSON。";
    return;
  }
  if (!templates || Array.isArray(templates) || typeof templates !== "object") {
    error.value = "Task 模板必须是 JSON 对象。";
    return;
  }
  const spec = { maxParallel: maxParallel.value, tasks };
  if (Object.keys(templates).length) spec.taskTemplates = templates;
  const body = {
    metadata: name.value.trim() ? { name: name.value.trim() } : {},
    spec,
  };
  submitting.value = true;
  try {
    const workflow = await api.createWorkflow(body);
    if (!disposed)
      router.push(
        workflow.metadata?.name
          ? { name: "WorkflowDetail", params: { name: workflow.metadata.name } }
          : { name: "WorkflowList" },
      );
  } catch (e) {
    if (!disposed) {
      const message = handleApiError(e);
      if (message !== null) error.value = "创建失败：" + message;
    }
  } finally {
    if (!disposed) submitting.value = false;
  }
}
</script>
