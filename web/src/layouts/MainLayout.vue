<template>
  <div class="app-layout">
    <aside class="sidebar">
      <h2>KubeTask</h2>
      <nav>
        <router-link to="/">仪表盘</router-link>
        <router-link to="/tasks">任务列表</router-link>
        <router-link to="/tasks/create">创建任务</router-link>
      </nav>
    </aside>
    <main class="content">
      <router-view />
    </main>
  </div>
</template>

<style>
.app-layout { display: flex; min-height: 100vh; }
.sidebar {
  width: 220px; background: #1e293b; color: #fff; padding: 20px; flex-shrink: 0;
}
.sidebar h2 { font-size: 18px; margin-bottom: 24px; }
.sidebar nav a {
  display: block; color: #94a3b8; text-decoration: none; padding: 8px 0;
  font-size: 14px;
}
.sidebar nav a:hover, .sidebar nav a.router-link-active {
  color: #fff;
}
.content { flex: 1; padding: 24px; overflow-y: auto; }
</style>
