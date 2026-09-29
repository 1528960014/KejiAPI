import { createRouter, createWebHistory } from 'vue-router'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', name: 'home', component: () => import('../views/HomeView.vue') },
    { path: '/chat', name: 'chat', component: () => import('../views/ChatView.vue') },
    { path: '/workbench', name: 'workbench', component: () => import('../views/WorkbenchView.vue') },
    { path: '/pricing', name: 'pricing', component: () => import('../views/PricingView.vue') },
    { path: '/console', name: 'console', component: () => import('../views/ConsoleView.vue') },
    { path: '/login', name: 'login', component: () => import('../views/LoginView.vue') },
  ],
})

export default router