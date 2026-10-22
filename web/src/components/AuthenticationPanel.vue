<template>
  <section class="authentication" aria-label="API 认证">
    <div class="auth-summary">
      <GlassButton
        ref="trigger"
        :label="hasAuthToken ? '已设置 Token' : 'API 认证'"
        aria-haspopup="dialog"
        :aria-expanded="expanded || authRequired"
        aria-controls="auth-settings-panel"
        @click="changeOpen(!expanded)"
      />
    </div>
    <PopupPanel
      id="auth-settings-panel"
      :open="expanded || authRequired"
      :anchor="trigger"
      panel-class="auth-popover"
      label="API 认证设置"
      @dismiss="changeOpen(false)"
    >
      <form id="auth-settings" class="auth-form" @submit.prevent="apply">
        <div class="auth-form-heading">
          <strong>API 认证</strong>
          <button
            v-if="!authRequired"
            type="button"
            class="icon-button"
            aria-label="关闭认证设置"
            @click="changeOpen(false)"
          >
            <AppIcon name="close" />
          </button>
        </div>
        <p v-if="authRequired" class="auth-error" role="alert">
          认证未通过，请输入有效的 ServiceAccount Token。
        </p>
        <label for="serviceaccount-token">ServiceAccount Token</label>
        <input
          id="serviceaccount-token"
          v-model="draft"
          type="password"
          autofocus
          autocomplete="off"
          autocapitalize="off"
          :spellcheck="false"
          placeholder="粘贴 Token（不含 Bearer 前缀）"
        />
        <p>
          仅在当前页面内存中保存，刷新后需重新输入。后端未开启认证时可留空。更换或清除后将重新加载当前视图。
        </p>
        <p v-if="error" class="auth-error" role="alert">{{ error }}</p>
        <div class="auth-form-actions">
          <GlassButton
            type="submit"
            tone="accent"
            label="应用 Token"
            :disabled="!draft.trim()"
          />
          <GlassButton v-if="hasAuthToken" label="清除 Token" @click="clear" />
        </div>
      </form>
    </PopupPanel>
  </section>
</template>

<script setup>
import { ref } from "vue";
import PopupPanel from "@/components/PopupPanel.vue";
import AppIcon from "@/components/AppIcon.vue";
import GlassButton from "@/components/GlassButton.vue";
import { authRequired, hasAuthToken, setAuthToken } from "@/api/auth.js";

const draft = ref("");
const expanded = ref(false);
const trigger = ref(null);
const error = ref("");
function changeOpen(value) {
  expanded.value = value || authRequired.value;
}

function apply() {
  try {
    setAuthToken(draft.value);
    draft.value = "";
    error.value = "";
    expanded.value = false;
  } catch (e) {
    error.value = e.message;
  }
}

function clear() {
  draft.value = "";
  error.value = "";
  expanded.value = false;
  setAuthToken("");
}
</script>
