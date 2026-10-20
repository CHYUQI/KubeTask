<template>
  <div class="app-layout">
    <header class="app-toolbar">
      <router-link to="/" class="brand" aria-label="KubeTask 首页"
        >KubeTask</router-link
      >
      <LiquidGlassNav />
      <AuthenticationPanel />
    </header>
    <main id="main-content" class="content">
      <router-view
        :key="
          String(route.name) +
          ':' +
          String(route.params.name || '') +
          ':' +
          authRevision
        "
      />
    </main>
  </div>
</template>
<script setup>
import { useRoute } from "vue-router";
import AuthenticationPanel from "@/components/AuthenticationPanel.vue";
import LiquidGlassNav from "@/components/LiquidGlassNav.vue";
import { authRevision } from "@/api/auth.js";
const route = useRoute();
</script>
