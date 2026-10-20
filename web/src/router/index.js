import { createRouter, createWebHashHistory } from 'vue-router'

const routes = [
  {
    path: '/',
    component: () => import('@/layouts/MainLayout.vue'),
    children: [
      { path: '', name: 'Dashboard', component: () => import('@/pages/Dashboard.vue') },
      { path: 'workflows', name: 'WorkflowList', component: () => import('@/pages/WorkflowList.vue') },
      { path: 'workflows/create', name: 'WorkflowCreate', component: () => import('@/pages/WorkflowCreate.vue') },
      { path: 'workflows/:name', name: 'WorkflowDetail', component: () => import('@/pages/WorkflowDetail.vue') },
      { path: 'tasks', name: 'TaskList', component: () => import('@/pages/TaskList.vue') },
      { path: 'tasks/create', name: 'TaskCreate', component: () => import('@/pages/TaskCreate.vue') },
      { path: 'tasks/:name', name: 'TaskDetail', component: () => import('@/pages/TaskDetail.vue') },
      { path: 'tasks/:name/logs', name: 'TaskLogs', component: () => import('@/pages/TaskLogs.vue') }
    ]
  }
]

export default createRouter({
  history: createWebHashHistory(),
  scrollBehavior: () => ({ top: 0 }),
  routes
})
