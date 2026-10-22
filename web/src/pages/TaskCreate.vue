<template>
  <div class="create-task page-compact">
    <PageHeading title="创建任务" back-to="/tasks" back-label="任务" />
    <GlassContainer v-if="error" class="error-msg">{{ error }}</GlassContainer>
    <NoCluster v-if="clusterError" :message="clusterError" />

    <div v-if="!clusterError" class="create-layout">
      <GlassContainer as="form" @submit.prevent="submit" class="form">
        <section class="form-section" aria-labelledby="task-basic-heading">
          <h2 id="task-basic-heading">基本信息</h2>
          <div class="form-row">
            <div class="form-group">
              <label for="task-name">名称 <span class="opt">可选</span></label>
              <input
                id="task-name"
                v-model="form.name"
                placeholder="留空自动生成"
              />
            </div>

            <div class="form-group">
              <BaseSelect
                id="task-type"
                v-model="form.type"
                label="类型"
                :options="taskTypes"
              />
            </div>
          </div>

          <div class="form-group" v-if="form.type === 'Cron'">
            <label for="task-schedule">Cron 表达式</label>
            <input
              id="task-schedule"
              v-model="form.schedule"
              placeholder="*/5 * * * *"
            />
            <p class="hint">标准 5 字段：分 时 日 月 周</p>
          </div>

          <div class="form-group" v-if="form.type === 'Cron'">
            <BaseSelect
              id="task-concurrency"
              v-model="form.concurrencyPolicy"
              label="并发策略"
              :options="concurrencyPolicies"
            />
          </div>

          <div class="form-group" v-if="form.type === 'Delay'">
            <label for="task-delay">延迟时长</label>
            <input id="task-delay" v-model="form.delay" placeholder="5m" />
            <p class="hint">格式：30s / 5m / 1h</p>
          </div>
        </section>

        <section class="form-section" aria-labelledby="task-container-heading">
          <h2 id="task-container-heading">容器</h2>
          <div class="form-group">
            <label for="task-image">镜像 <span class="req">*</span></label>
            <input
              id="task-image"
              v-model="form.image"
              placeholder="busybox:latest"
              required
            />
          </div>

          <div class="form-row">
            <div class="form-group">
              <label for="task-command">命令 Command</label>
              <input
                id="task-command"
                v-model="form.command"
                placeholder="echo,hello"
              />
            </div>
            <div class="form-group">
              <label for="task-args">参数 Args</label>
              <input
                id="task-args"
                v-model="form.args"
                placeholder="arg1,arg2"
              />
            </div>
          </div>

          <div class="form-group">
            <label for="task-env">环境变量 <span class="opt">可选</span></label>
            <textarea
              id="task-env"
              v-model="form.envText"
              rows="3"
              placeholder="ENV=production&#10;LOG_LEVEL=debug"
            ></textarea>
            <p class="hint">每行一个 key=value。</p>
          </div>
        </section>

        <details class="form-disclosure">
          <summary>运行选项</summary>
          <div class="form-row form-row-three">
            <div class="form-group">
              <label for="task-retries">失败重试次数</label>
              <input
                id="task-retries"
                v-model.number="form.backoffLimit"
                type="number"
                min="0"
              />
            </div>
            <div class="form-group">
              <label for="task-deadline">超时时间 (秒)</label>
              <input
                id="task-deadline"
                v-model.number="form.activeDeadlineSeconds"
                type="number"
                min="0"
              />
            </div>
            <div class="form-group">
              <label for="task-ttl">完成后保留 (秒)</label>
              <input
                id="task-ttl"
                v-model.number="form.ttlSecondsAfterFinished"
                type="number"
                min="0"
              />
            </div>
          </div>
        </details>

        <div class="form-actions">
          <GlassButton
            type="submit"
            tone="accent"
            :label="submitting ? '创建中…' : '创建任务'"
            :disabled="submitting"
          />
        </div>
      </GlassContainer>
    </div>
  </div>
</template>

<script setup>
import GlassContainer from "@/components/GlassContainer.vue";
import { reactive, ref } from "vue";
import PageHeading from "@/components/PageHeading.vue";
import GlassButton from "@/components/GlassButton.vue";
import BaseSelect from "@/components/BaseSelect.vue";
import { taskTypes, concurrencyPolicies } from "@/components/select-options.js";
import { useRouter } from "vue-router";
import { api } from "@/api";
import { useClusterStatus } from "@/composables/cluster";
import NoCluster from "@/components/NoCluster.vue";

const router = useRouter();
const { clusterError, handleApiError } = useClusterStatus();
const error = ref("");
const submitting = ref(false);

const form = reactive({
  name: "",
  type: "OneTime",
  schedule: "",
  concurrencyPolicy: "Allow",
  delay: "",
  image: "",
  command: "",
  args: "",
  envText: "",
  backoffLimit: 3,
  activeDeadlineSeconds: null,
  ttlSecondsAfterFinished: 60,
});

async function submit() {
  if (!form.image.trim()) {
    error.value = "镜像为必填字段";
    return;
  }
  if (form.type === "Cron" && !form.schedule.trim()) {
    error.value = "Cron 类型需要填写 Cron 表达式";
    return;
  }
  if (form.type === "Delay" && !form.delay.trim()) {
    error.value = "Delay 类型需要填写延迟时长";
    return;
  }

  submitting.value = true;
  error.value = "";

  try {
    const spec = {
      type: form.type,
      image: form.image.trim(),
      command: parseList(form.command),
      args: parseList(form.args),
      backoffLimit: form.backoffLimit || 3,
    };

    if (form.type === "Cron") {
      spec.schedule = form.schedule.trim();
      spec.concurrencyPolicy = form.concurrencyPolicy;
    }
    if (form.type === "Delay") spec.delay = form.delay.trim();
    if (form.activeDeadlineSeconds)
      spec.activeDeadlineSeconds = form.activeDeadlineSeconds;
    if (form.ttlSecondsAfterFinished)
      spec.ttlSecondsAfterFinished = form.ttlSecondsAfterFinished;

    const envLines = form.envText.trim().split("\n").filter(Boolean);
    if (envLines.length > 0) {
      spec.env = envLines.map((line) => {
        const [name, ...rest] = line.split("=");
        return { name: name.trim(), value: rest.join("=").trim() };
      });
    }

    const body = {
      metadata: form.name.trim() ? { name: form.name.trim() } : {},
      spec,
    };

    await api.createTask(body);
    router.push("/tasks");
  } catch (e) {
    if (handleApiError(e) !== null) {
      error.value = "创建失败: " + handleApiError(e);
    }
  } finally {
    submitting.value = false;
  }
}

function parseList(s) {
  if (!s || !s.trim()) return null;
  return s
    .split(",")
    .map((x) => x.trim())
    .filter(Boolean);
}
</script>
