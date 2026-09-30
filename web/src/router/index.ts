import { createRouter, createWebHistory } from 'vue-router'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', redirect: '/home' },
    { path: '/home', name: 'home', component: () => import('../views/HomeView.vue') },
    { path: '/chat', name: 'chat', component: () => import('../views/ChatView.vue') },
    { path: '/models', name: 'models', component: () => import('../views/ModelSquareView.vue') },
    { path: '/rankings', name: 'rankings', component: () => import('../views/RankingsView.vue') },
    { path: '/workbench', name: 'workbench', component: () => import('../views/WorkbenchView.vue') },
    { path: '/drama', name: 'drama', component: () => import('../views/DramaView.vue') },
    { path: '/pricing', name: 'pricing', component: () => import('../views/PricingView.vue') },
    { path: '/recharge', name: 'recharge', component: () => import('../views/RechargeView.vue') },
    { path: '/console', name: 'console', component: () => import('../views/ConsoleView.vue') },
    { path: '/orgs', name: 'orgs', component: () => import('../views/OrgsView.vue') },
    { path: '/admin', name: 'admin', component: () => import('../views/AdminView.vue') },
    { path: '/login', name: 'login', component: () => import('../views/LoginView.vue') },
  ],
})

export default router