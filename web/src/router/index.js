import { createRouter, createWebHashHistory } from 'vue-router'

const routes = [
  {
    path: '/',
    component: () => import('@/layouts/MainLayout.vue'),
    children: [
      { path: '', name: 'Dashboard', component: () => import('@/pages/Dashboard.vue') },
      { path: 'tasks', name: 'TaskList', component: () => import('@/pages/TaskList.vue') },
      { path: 'tasks/create', name: 'TaskCreate', component: () => import('@/pages/TaskCreate.vue') },
      { path: 'tasks/:name', name: 'TaskDetail', component: () => import('@/pages/TaskDetail.vue') },
      { path: 'tasks/:name/logs', name: 'TaskLogs', component: () => import('@/pages/TaskLogs.vue') }
    ]
  }
]

export default createRouter({
  history: createWebHashHistory(),
  routes
})
